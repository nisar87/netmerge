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
// comes from QueryFullProcessImageNameW.

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

// One table per kind of socket (TCP/UDP, IPv4/IPv6). A new connection's socket is usually not in the last read:
// only its own kind is read again, at most every 2 ms, so a burst of new connections shares one read.
type ownerTable struct {
	at time.Time
	m  map[netip.AddrPort]uint32 // UDP sockets are often bound to 0.0.0.0 / ::: those are found by port
}

var owners struct {
	mu  sync.Mutex
	t   [4]ownerTable // tcp4, tcp6, udp4, udp6
	buf []byte
}

func readTable(proc *syscall.LazyProc, family uint32, class uint32) []byte {
	size := uint32(len(owners.buf))
	if size < 64*1024 {
		size = 64 * 1024
	}
	for i := 0; i < 4; i++ {
		if len(owners.buf) < int(size) {
			owners.buf = make([]byte, size)
		}
		size = uint32(len(owners.buf))
		r, _, _ := proc.Call(uintptr(unsafe.Pointer(&owners.buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), uintptr(class), 0)
		if r == 0 {
			return owners.buf[:size]
		}
		if r != 122 { // ERROR_INSUFFICIENT_BUFFER
			return nil
		}
		size += 16 * 1024
	}
	return nil
}

func portOf(b []byte) uint16 { return binary.BigEndian.Uint16(b[:2]) }

// refresh reads one kind of socket table (called with owners.mu held).
func refresh(k int) {
	m := map[netip.AddrPort]uint32{}
	switch k {
	case 0: // MIB_TCPROW_OWNER_PID: state, localAddr, localPort, remoteAddr, remotePort, pid (6 x 4 bytes)
		if b := readTable(procGetExtendedTcpTable, afInet, tcpTableOwnerPidAll); len(b) >= 4 {
			n := int(binary.LittleEndian.Uint32(b))
			for i := 0; i < n && 4+(i+1)*24 <= len(b); i++ {
				r := b[4+i*24:]
				m[netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[4:8])), portOf(r[8:12]))] = binary.LittleEndian.Uint32(r[20:24])
			}
		}
	case 1: // MIB_TCP6ROW_OWNER_PID: localAddr[16], localScope, localPort, remoteAddr[16], remoteScope, remotePort, state, pid
		if b := readTable(procGetExtendedTcpTable, afInet6, tcpTableOwnerPidAll); len(b) >= 4 {
			n := int(binary.LittleEndian.Uint32(b))
			for i := 0; i < n && 4+(i+1)*56 <= len(b); i++ {
				r := b[4+i*56:]
				m[netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[0:16])).Unmap(), portOf(r[20:24]))] = binary.LittleEndian.Uint32(r[52:56])
			}
		}
	case 2: // MIB_UDPROW_OWNER_PID: localAddr, localPort, pid
		if b := readTable(procGetExtendedUdpTable, afInet, udpTableOwnerPid); len(b) >= 4 {
			n := int(binary.LittleEndian.Uint32(b))
			for i := 0; i < n && 4+(i+1)*12 <= len(b); i++ {
				r := b[4+i*12:]
				m[netip.AddrPortFrom(netip.AddrFrom4([4]byte(r[0:4])), portOf(r[4:8]))] = binary.LittleEndian.Uint32(r[8:12])
			}
		}
	case 3: // MIB_UDP6ROW_OWNER_PID: localAddr[16], localScope, localPort, pid
		if b := readTable(procGetExtendedUdpTable, afInet6, udpTableOwnerPid); len(b) >= 4 {
			n := int(binary.LittleEndian.Uint32(b))
			for i := 0; i < n && 4+(i+1)*28 <= len(b); i++ {
				r := b[4+i*28:]
				m[netip.AddrPortFrom(netip.AddrFrom16([16]byte(r[0:16])).Unmap(), portOf(r[20:24]))] = binary.LittleEndian.Uint32(r[24:28])
			}
		}
	}
	owners.t[k] = ownerTable{at: time.Now(), m: m}
}

func lookupOwner(network string, ip netip.Addr, port uint16) (uint32, bool) {
	ip = ip.Unmap()
	k := 0
	if network == "udp" {
		k = 2
	}
	kinds := []int{k, k + 1} // the socket's own family first; a dual-stack (IPv6) socket can carry IPv4 too
	if !ip.Is4() {
		kinds = []int{k + 1}
	}
	try := func(m map[netip.AddrPort]uint32) (uint32, bool) {
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
	for _, k := range kinds {
		if pid, ok := try(owners.t[k].m); ok {
			return pid, true
		}
	}
	for _, k := range kinds {
		if wait := 2*time.Millisecond - time.Since(owners.t[k].at); wait > 0 {
			time.Sleep(wait)
		}
		refresh(k)
		if pid, ok := try(owners.t[k].m); ok {
			return pid, true
		}
	}
	return 0, false
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
