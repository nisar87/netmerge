//go:build windows

package main

import (
	"fmt"
	"net/netip"
	"strconv"
	"syscall"
	"unsafe"
)

var pGetAdaptersAddresses = iphlpapi.NewProc("GetAdaptersAddresses")

// listAdaptersAPI lists the adapters that are up and have an IPv4 default gateway, straight from Windows
// (GetAdaptersAddresses: about a millisecond). The PowerShell scan it replaces took seconds and sometimes
// failed ("adapter scan failed: exit status 0xffff0000" in real logs).
// IP_ADAPTER_ADDRESSES_LH offsets (64-bit): IfIndex 4, Next 8, FirstUnicastAddress 24, Description 64,
// FriendlyName 72, OperStatus 104, TransmitLinkSpeed 184, FirstGatewayAddress 208.
func listAdaptersAPI() ([]Adapter, error) {
	const flags = 0x80 | 0x2 | 0x4 | 0x8 // include gateways; skip anycast, multicast, DNS servers
	size := uint32(32 << 10)
	var buf []byte
	for i := 0; i < 5; i++ {
		buf = make([]byte, size)
		r, _, _ := pGetAdaptersAddresses.Call(2 /* AF_INET */, flags, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r == 0 {
			break
		}
		if r != 111 { // ERROR_BUFFER_OVERFLOW
			return nil, fmt.Errorf("GetAdaptersAddresses: %w", syscall.Errno(r))
		}
		buf = nil
	}
	if buf == nil {
		return nil, fmt.Errorf("GetAdaptersAddresses: list keeps growing")
	}
	var out []Adapter
	for p := unsafe.Pointer(&buf[0]); p != nil; p = *(*unsafe.Pointer)(unsafe.Add(p, 8)) {
		if *(*uint32)(unsafe.Add(p, 104)) != 1 { // OperStatus: up
			continue
		}
		gw := firstIPv4(*(*unsafe.Pointer)(unsafe.Add(p, 208)))
		ip := firstIPv4(*(*unsafe.Pointer)(unsafe.Add(p, 24)))
		if !gw.IsValid() || !ip.IsValid() {
			continue
		}
		out = append(out, Adapter{
			Name: utf16At(*(*unsafe.Pointer)(unsafe.Add(p, 72))), Index: int(*(*uint32)(unsafe.Add(p, 4))),
			Desc: utf16At(*(*unsafe.Pointer)(unsafe.Add(p, 64))), IP: ip.String(), Gateway: gw.String(),
			LinkRate: linkRateText(*(*uint64)(unsafe.Add(p, 184))), Up: true,
		})
	}
	return out, nil
}

// firstIPv4 walks a list of IP_ADAPTER_UNICAST_ADDRESS / IP_ADAPTER_GATEWAY_ADDRESS (both: Next at 8, then a
// SOCKET_ADDRESS whose sockaddr pointer is at 16) and returns the first IPv4 address.
func firstIPv4(p unsafe.Pointer) netip.Addr {
	for ; p != nil; p = *(*unsafe.Pointer)(unsafe.Add(p, 8)) {
		sa := *(*unsafe.Pointer)(unsafe.Add(p, 16))
		if sa != nil && *(*uint16)(sa) == 2 { // AF_INET
			return netip.AddrFrom4(*(*[4]byte)(unsafe.Add(sa, 4)))
		}
	}
	return netip.Addr{}
}

func utf16At(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*uint16)(unsafe.Add(p, n*2)) != 0 && n < 1024 {
		n++
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(p), n))
}

// linkRateText writes the speed the way Windows shows it ("1 Gbps", "866.7 Mbps").
func linkRateText(bps uint64) string {
	switch {
	case bps == 0 || bps == ^uint64(0):
		return ""
	case bps >= 1e9:
		return strconv.FormatFloat(float64(bps)/1e9, 'f', -1, 64) + " Gbps"
	case bps >= 1e6:
		return strconv.FormatFloat(float64(bps)/1e6, 'f', 1, 64) + " Mbps"
	default:
		return strconv.FormatFloat(float64(bps)/1e3, 'f', 1, 64) + " Kbps"
	}
}
