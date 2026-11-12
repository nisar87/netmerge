package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// On Windows a socket is tied to an adapter with IP_UNICAST_IF / IPV6_UNICAST_IF: traffic then leaves through that
// adapter even though the PC's routes point everywhere at NetMerge's own virtual adapter.

const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

var (
	procAliasToLuid = modIphlpapi.NewProc("ConvertInterfaceAliasToLuid")
	ifCacheMu       sync.Mutex
	ifCache         = map[string]ifCached{}
)

type ifCached struct {
	idx int
	at  time.Time
}

// ifIndex finds an adapter's number by its name. Every new connection needs it, so it is asked directly
// (ConvertInterfaceAliasToLuid, microseconds) instead of listing all adapters (net.InterfaceByName, about two
// milliseconds on Windows), and remembered for two seconds.
func ifIndex(iface string) (int, error) {
	ifCacheMu.Lock()
	c, ok := ifCache[iface]
	ifCacheMu.Unlock()
	if ok && time.Since(c.at) < 2*time.Second {
		return c.idx, nil
	}
	idx := 0
	if a, err := syscall.UTF16PtrFromString(iface); err == nil {
		var luid uint64
		var i uint32
		if r, _, _ := procAliasToLuid.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(&luid))); r == 0 {
			if r, _, _ := procConvertLuidToIndex.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&i))); r == 0 {
				idx = int(i)
			}
		}
	}
	if idx == 0 {
		ni, err := net.InterfaceByName(iface)
		if err != nil {
			return 0, fmt.Errorf("adapter %q: %w", iface, err)
		}
		idx = ni.Index
	}
	ifCacheMu.Lock()
	ifCache[iface] = ifCached{idx, time.Now()}
	ifCacheMu.Unlock()
	return idx, nil
}

func setUnicastIf(fd uintptr, idx int, v6 bool) error {
	h := syscall.Handle(fd)
	if v6 {
		v := uint32(idx)
		return syscall.SetsockoptInt(h, syscall.IPPROTO_IPV6, ipv6UnicastIf, int(v))
	}
	// IPv4 wants the index in network byte order
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(idx))
	v := *(*uint32)(unsafe.Pointer(&b[0]))
	return syscall.SetsockoptInt(h, syscall.IPPROTO_IP, ipUnicastIf, int(v))
}

func bindDialer(d *net.Dialer, iface string, dst netip.Addr) error {
	if iface == "" {
		return nil
	}
	idx, err := ifIndex(iface)
	if err != nil {
		return err
	}
	v6 := dst.Is6() && !dst.Is4In6()
	d.Control = func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) { serr = setUnicastIf(fd, idx, v6) })
		if err != nil {
			return err
		}
		return serr
	}
	return nil
}

func listenUDPOn(iface string) (*net.UDPConn, error) {
	lc := net.ListenConfig{}
	if iface != "" {
		idx, err := ifIndex(iface)
		if err != nil {
			return nil, err
		}
		lc.Control = func(network, address string, c syscall.RawConn) error {
			var serr error
			err := c.Control(func(fd uintptr) {
				serr = setUnicastIf(fd, idx, false)
				if serr == nil {
					_ = setUnicastIf(fd, idx, true) // dual-stack socket: best effort for IPv6
				}
			})
			if err != nil {
				return err
			}
			return serr
		}
	}
	pc, err := lc.ListenPacket(context.Background(), "udp", ":0")
	if err != nil {
		return nil, err
	}
	return pc.(*net.UDPConn), nil
}
