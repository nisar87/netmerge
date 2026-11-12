package engine

import (
	"context"
	"net"
	"net/netip"
	"syscall"
)

// On Linux (used for tests and development) a socket is tied to an adapter with SO_BINDTODEVICE.

func bindDialer(d *net.Dialer, iface string, _ netip.Addr) error {
	if iface == "" {
		return nil
	}
	d.Control = func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) { serr = syscall.BindToDevice(int(fd), iface) })
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
		lc.Control = func(network, address string, c syscall.RawConn) error {
			var serr error
			err := c.Control(func(fd uintptr) { serr = syscall.BindToDevice(int(fd), iface) })
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
