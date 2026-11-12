package engine

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// The virtual adapter's addresses, settings and routes are set with the IP Helper API directly (microseconds per
// call) rather than with netsh (a new program per change, about a tenth of a second each): capture is ready
// moments after the engine starts. Row layouts are those of netioapi.h on 64-bit Windows.

var (
	procInitUnicastRow   = modIphlpapi.NewProc("InitializeUnicastIpAddressEntry")
	procCreateUnicast    = modIphlpapi.NewProc("CreateUnicastIpAddressEntry")
	procInitForwardRow   = modIphlpapi.NewProc("InitializeIpForwardEntry")
	procCreateForward2   = modIphlpapi.NewProc("CreateIpForwardEntry2")
	procDeleteForward2   = modIphlpapi.NewProc("DeleteIpForwardEntry2")
	procInitIfRow        = modIphlpapi.NewProc("InitializeIpInterfaceEntry")
	procGetIfEntry       = modIphlpapi.NewProc("GetIpInterfaceEntry")
	procSetIfEntry       = modIphlpapi.NewProc("SetIpInterfaceEntry")
	procIndexToLuid      = modIphlpapi.NewProc("ConvertInterfaceIndexToLuid")
	modAdvapi32          = syscall.NewLazyDLL("advapi32.dll")
	procRegEnumValueW    = modAdvapi32.NewProc("RegEnumValueW")
	errObjectExists      = syscall.Errno(5010) // ERROR_OBJECT_ALREADY_EXISTS
	errNotFoundIphlp     = syscall.Errno(1168) // ERROR_NOT_FOUND
	errInvalidParamIphlp = syscall.Errno(87)
)

// putSockaddr writes a SOCKADDR_INET (28 bytes) for a.
func putSockaddr(b []byte, a netip.Addr) {
	for i := range b[:28] {
		b[i] = 0
	}
	if a.Is4() {
		binary.LittleEndian.PutUint16(b[0:2], 2) // AF_INET
		v := a.As4()
		copy(b[4:8], v[:])
		return
	}
	binary.LittleEndian.PutUint16(b[0:2], 23) // AF_INET6
	v := a.As16()
	copy(b[8:24], v[:])
}

func iphlpErr(name string, r uintptr) error {
	if r == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w", name, syscall.Errno(r))
}

// addAddress gives the adapter an address (MIB_UNICASTIPADDRESS_ROW, 80 bytes).
func addAddress(luid uint64, p netip.Prefix) error {
	var row [80]byte
	procInitUnicastRow.Call(uintptr(unsafe.Pointer(&row[0])))
	putSockaddr(row[0:28], p.Addr())
	binary.LittleEndian.PutUint64(row[32:40], luid)
	row[60] = byte(p.Bits())                     // OnLinkPrefixLength
	binary.LittleEndian.PutUint32(row[64:68], 4) // DadState: preferred (no duplicate check wait)
	r, _, _ := procCreateUnicast.Call(uintptr(unsafe.Pointer(&row[0])))
	if syscall.Errno(r) == errObjectExists {
		return nil
	}
	return iphlpErr("CreateUnicastIpAddressEntry", r)
}

// setInterface sets the adapter's MTU and metric for one address family (MIB_IPINTERFACE_ROW, 168 bytes).
func setInterface(luid uint64, v6 bool, mtu, metric int) error {
	var row [168]byte
	procInitIfRow.Call(uintptr(unsafe.Pointer(&row[0])))
	fam := uint16(2)
	if v6 {
		fam = 23
	}
	binary.LittleEndian.PutUint16(row[0:2], fam)
	binary.LittleEndian.PutUint64(row[8:16], luid)
	if r, _, _ := procGetIfEntry.Call(uintptr(unsafe.Pointer(&row[0]))); r != 0 {
		return iphlpErr("GetIpInterfaceEntry", r)
	}
	row[44] = 0                                                 // UseAutomaticMetric: off
	binary.LittleEndian.PutUint32(row[148:152], uint32(metric)) // Metric
	if mtu > 0 {
		binary.LittleEndian.PutUint32(row[152:156], uint32(mtu)) // NlMtu
	}
	if !v6 {
		row[144] = 0 // SitePrefixLength must be 0 for IPv4
	}
	r, _, _ := procSetIfEntry.Call(uintptr(unsafe.Pointer(&row[0])))
	return iphlpErr("SetIpInterfaceEntry", r)
}

// routeRow fills a MIB_IPFORWARD_ROW2 (104 bytes).
func routeRow(luid uint64, ifIndex int, p netip.Prefix, nextHop netip.Addr, metric int) [104]byte {
	var row [104]byte
	procInitForwardRow.Call(uintptr(unsafe.Pointer(&row[0])))
	binary.LittleEndian.PutUint64(row[0:8], luid)
	binary.LittleEndian.PutUint32(row[8:12], uint32(ifIndex))
	putSockaddr(row[12:40], p.Masked().Addr())
	row[40] = byte(p.Bits())
	if !nextHop.IsValid() {
		if p.Addr().Is4() {
			nextHop = netip.IPv4Unspecified()
		} else {
			nextHop = netip.IPv6Unspecified()
		}
	}
	putSockaddr(row[44:72], nextHop)
	binary.LittleEndian.PutUint32(row[84:88], uint32(metric))
	binary.LittleEndian.PutUint32(row[88:92], 3) // Protocol: MIB_IPPROTO_NETMGMT (a static route)
	return row
}

func addRoute(luid uint64, ifIndex int, p netip.Prefix, nextHop netip.Addr, metric int) error {
	row := routeRow(luid, ifIndex, p, nextHop, metric)
	r, _, _ := procCreateForward2.Call(uintptr(unsafe.Pointer(&row[0])))
	if syscall.Errno(r) == errObjectExists {
		return nil
	}
	return iphlpErr("CreateIpForwardEntry2", r)
}

func delRoute(luid uint64, ifIndex int, p netip.Prefix, nextHop netip.Addr) {
	row := routeRow(luid, ifIndex, p, nextHop, 0)
	procDeleteForward2.Call(uintptr(unsafe.Pointer(&row[0])))
}

func luidOf(ifIndex int) uint64 {
	var luid uint64
	procIndexToLuid.Call(uintptr(ifIndex), uintptr(unsafe.Pointer(&luid)))
	return luid
}

// firewallRuleOK tells whether the inbound rule for the engine already exists for this program and the adapter's
// address (read from the firewall's own registry list; adding it again with netsh takes a second).
func firewallRuleOK(exe string) bool {
	const path = `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\FirewallRules`
	var k syscall.Handle
	p, _ := syscall.UTF16PtrFromString(path)
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, p, 0, syscall.KEY_READ, &k) != nil {
		return false
	}
	defer syscall.RegCloseKey(k)
	want := []string{"|name=netmerge engine|", "|app=" + strings.ToLower(exe) + "|", "|la4=" + tunGW4.String() + "|", "|action=allow|", "|active=true|", "|dir=in|"}
	name := make([]uint16, 512)
	data := make([]byte, 8192)
	for i := uint32(0); i < 100000; i++ {
		nl, dl := uint32(len(name)), uint32(len(data))
		var typ uint32
		r, _, _ := procRegEnumValueW.Call(uintptr(k), uintptr(i), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&nl)),
			0, uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(&dl)))
		if r == 259 { // ERROR_NO_MORE_ITEMS
			return false
		}
		if r != 0 || typ != 1 || dl < 2 { // REG_SZ only
			continue
		}
		u := unsafe.Slice((*uint16)(unsafe.Pointer(&data[0])), dl/2)
		v := strings.ToLower(syscall.UTF16ToString(u))
		ok := true
		for _, w := range want {
			if !strings.Contains(v, w) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// ensureFirewall makes sure Windows lets the rewritten connections in (they arrive from the adapter, a "public"
// network to Windows, at the engine's listener). The rule stays between runs, so this is quick after the first.
func (t *tunStack) ensureFirewall() {
	exe, _ := os.Executable()
	exe = longPath(exe)
	if exe != "" && firewallRuleOK(exe) {
		return
	}
	netsh("advfirewall", "firewall", "delete", "rule", "name=NetMerge engine")
	local := "localip=" + tunGW4.String() + "," + tunGW6.String()
	if exe != "" && netsh("advfirewall", "firewall", "add", "rule", "name=NetMerge engine", "dir=in", "action=allow",
		"program="+exe, local, "enable=yes", "profile=any") == nil {
		return
	}
	if err := netsh("advfirewall", "firewall", "add", "rule", "name=NetMerge engine", "dir=in", "action=allow",
		local, "enable=yes", "profile=any"); err != nil {
		t.e.log.Warnf("[TUN] firewall: %v", firstLine(err.Error()))
	}
}
