package engine

import (
	"encoding/binary"
	"net/netip"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Windows: GetExtendedTcpTable / GetExtendedUdpTable list every socket with its process id; the program's path
// comes from QueryFullProcessImageNameW. The tables are read again when a socket is not in them (at most every 50 ms) and searched from memory.

var (
	modIphlpapi             = syscall.NewLazyDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modIphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modIphlpapi.NewProc("GetExtendedUdpTable")
	modKernel32             = syscall.NewLazyDLL("kernel32.dll")
	procQueryFullImageName  = modKernel32.NewProc("QueryFullProcessImageNameW")
)

const (
	afInet              = 2
	afInet6             = 23
	tcpTableOwnerPidAll = 5
	udpTableOwnerPid    = 1
	processQueryLimited = 0x1000
)

type ownerTables struct {
	mu  sync.Mutex
	at  time.Time
	tcp map[netip.AddrPort]uint32
	udp map[netip.AddrPort]uint32 // UDP sockets are often bound to 0.0.0.0: keyed by port with the unspecified address too
}

var owners ownerTables

func readTable(proc *syscall.LazyProc, family uint32, class uint32) []byte {
	size := uint32(64 * 1024)
	for i := 0; i < 4; i++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), uintptr(class), 0)
		if r == 0 {
			return buf[:size]
		}
		if r != 122 { // ERROR_INSUFFICIENT_BUFFER
			return nil
		}
		size += 16 * 1024
	}
	return nil
}

func portOf(b []byte) uint16 { return binary.BigEndian.Uint16(b[:2]) }

func (o *ownerTables) refresh() {
	tcp, udp := map[netip.AddrPort]uint32{}, map[netip.AddrPort]uint32{}
	// MIB_TCPROW_OWNER_PID: state, localAddr, localPort, remoteAddr, remotePort, pid (6 x 4 bytes)
	if b := readTable(procGetExtendedTcpTable, afInet, tcpTableOwnerPidAll); len(b) >= 4 {
		n := int(binary.LittleEndian.Uint32(b))
		for i := 0; i < n && 4+(i+1)*24 <= len(b); i++ {
			r := b[4+i*24:]
			a := netip.AddrFrom4([4]byte(r[4:8]))
			tcp[netip.AddrPortFrom(a, portOf(r[8:12]))] = binary.LittleEndian.Uint32(r[20:24])
		}
	}
	// MIB_TCP6ROW_OWNER_PID: localAddr[16], localScope, localPort, remoteAddr[16], remoteScope, remotePort, state, pid
	if b := readTable(procGetExtendedTcpTable, afInet6, tcpTableOwnerPidAll); len(b) >= 4 {
		n := int(binary.LittleEndian.Uint32(b))
		for i := 0; i < n && 4+(i+1)*56 <= len(b); i++ {
			r := b[4+i*56:]
			a := netip.AddrFrom16([16]byte(r[0:16])).Unmap()
			tcp[netip.AddrPortFrom(a, portOf(r[20:24]))] = binary.LittleEndian.Uint32(r[52:56])
		}
	}
	// MIB_UDPROW_OWNER_PID: localAddr, localPort, pid
	if b := readTable(procGetExtendedUdpTable, afInet, udpTableOwnerPid); len(b) >= 4 {
		n := int(binary.LittleEndian.Uint32(b))
		for i := 0; i < n && 4+(i+1)*12 <= len(b); i++ {
			r := b[4+i*12:]
			a := netip.AddrFrom4([4]byte(r[0:4]))
			udp[netip.AddrPortFrom(a, portOf(r[4:8]))] = binary.LittleEndian.Uint32(r[8:12])
		}
	}
	// MIB_UDP6ROW_OWNER_PID: localAddr[16], localScope, localPort, pid
	if b := readTable(procGetExtendedUdpTable, afInet6, udpTableOwnerPid); len(b) >= 4 {
		n := int(binary.LittleEndian.Uint32(b))
		for i := 0; i < n && 4+(i+1)*28 <= len(b); i++ {
			r := b[4+i*28:]
			a := netip.AddrFrom16([16]byte(r[0:16])).Unmap()
			udp[netip.AddrPortFrom(a, portOf(r[20:24]))] = binary.LittleEndian.Uint32(r[24:28])
		}
	}
	o.tcp, o.udp, o.at = tcp, udp, time.Now()
}

func lookupOwner(network string, ip netip.Addr, port uint16) (uint32, bool) {
	ip = ip.Unmap()
	try := func() (uint32, bool) {
		m := owners.tcp
		if network == "udp" {
			m = owners.udp
		}
		if pid, ok := m[netip.AddrPortFrom(ip, port)]; ok {
			return pid, true
		}
		for _, any := range []netip.Addr{netip.IPv4Unspecified(), netip.IPv6Unspecified()} {
			if pid, ok := m[netip.AddrPortFrom(any, port)]; ok {
				return pid, true
			}
		}
		return 0, false
	}
	owners.mu.Lock()
	defer owners.mu.Unlock()
	if owners.tcp != nil {
		if pid, ok := try(); ok {
			return pid, true
		}
	}
	// a socket opened just after the last read is not in it: read again, at most every 50 ms (a read takes about a
	// millisecond), waiting out the rest of that interval instead of giving up on the program's name
	if wait := 50*time.Millisecond - time.Since(owners.at); wait > 0 {
		time.Sleep(wait)
	}
	owners.refresh()
	return try()
}

func processPath(pid uint32) string {
	if pid == 0 || pid == 4 {
		return "System"
	}
	h, err := syscall.OpenProcess(processQueryLimited, false, pid)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	r, _, _ := procQueryFullImageName.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}
