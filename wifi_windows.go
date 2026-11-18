//go:build windows

package main

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	wlanapi             = syscall.NewLazyDLL("wlanapi.dll")
	pWlanOpenHandle     = wlanapi.NewProc("WlanOpenHandle")
	pWlanEnumInterfaces = wlanapi.NewProc("WlanEnumInterfaces")
	pWlanQueryInterface = wlanapi.NewProc("WlanQueryInterface")
	pWlanFreeMemory     = wlanapi.NewProc("WlanFreeMemory")
	wlanMu              sync.Mutex
	wlanHandle          uintptr
	wlanWarned          bool
)

// wifiNames returns the name (SSID) of the network each connected Wi-Fi adapter is on, keyed by the adapter's
// lower-case "{GUID}". It asks the WLAN service directly (no netsh, no process). Windows 11 24H2 and later only
// give the name to apps that may use location; without that the map is empty and the name is simply not shown.
// WLAN_INTERFACE_INFO_LIST: count 0, items from 8, each 532 bytes (GUID 16, description 512, state 4).
// WLAN_CONNECTION_ATTRIBUTES: profile name at 8 (WCHAR[256]), DOT11_SSID at 520 (length 4, bytes 32).
func wifiNames() map[string]string {
	if wlanapi.Load() != nil {
		return nil
	}
	wlanMu.Lock()
	defer wlanMu.Unlock()
	if wlanHandle == 0 {
		var ver uint32
		if r, _, _ := pWlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&ver)), uintptr(unsafe.Pointer(&wlanHandle))); r != 0 {
			wlanHandle = 0
			return nil
		}
	}
	var list unsafe.Pointer
	if r, _, _ := pWlanEnumInterfaces.Call(wlanHandle, 0, uintptr(unsafe.Pointer(&list))); r != 0 || list == nil {
		wlanHandle = 0 // the service may have restarted; open again next time
		return nil
	}
	defer pWlanFreeMemory.Call(uintptr(list))
	out := map[string]string{}
	n := *(*uint32)(list)
	for i := uint32(0); i < n && i < 16; i++ {
		item := unsafe.Add(list, 8+uintptr(i)*532)
		if *(*uint32)(unsafe.Add(item, 528)) != 1 { // wlan_interface_state_connected
			continue
		}
		var size uint32
		var data unsafe.Pointer
		r, _, _ := pWlanQueryInterface.Call(wlanHandle, uintptr(item), 7 /* current_connection */, 0,
			uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0)
		if r != 0 || data == nil || size < 556 {
			if r == 5 && !wlanWarned { // ERROR_ACCESS_DENIED: location is off for apps
				wlanWarned = true
				logf("Wi-Fi name not available: Windows only gives it to apps allowed to use location")
			}
			continue
		}
		name := ssidText(data)
		if name == "" {
			name = utf16At(unsafe.Add(data, 8))
		}
		pWlanFreeMemory.Call(uintptr(data))
		if name != "" {
			out[strings.ToLower(guidText(item))] = name
		}
	}
	return out
}

func ssidText(conn unsafe.Pointer) string {
	n := *(*uint32)(unsafe.Add(conn, 520))
	if n == 0 || n > 32 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Add(conn, 524)), n))
}

// guidText writes a GUID the way GetAdaptersAddresses names adapters: {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}.
func guidText(p unsafe.Pointer) string {
	b := unsafe.Slice((*byte)(p), 16)
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		*(*uint32)(p), *(*uint16)(unsafe.Add(p, 4)), *(*uint16)(unsafe.Add(p, 6)),
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}
