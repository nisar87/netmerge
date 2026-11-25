package engine

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Windows VPN capture (test). Instead of the Wintun adapter, Windows' own VPN platform catches the PC's traffic and
// NetMerge's VPN plug-in (a small Windows app package) passes every packet to the engine over 127.0.0.1, one IP
// packet per UDP datagram. The plug-in first sends "\x00NMVPN1 <token>" (an IP packet never starts with a zero
// byte); only the socket that sent the right token is listened to, so no other program on the PC can slip
// packets in.

const vpnHelloPrefix = "\x00NMVPN1 "

type loopDev struct {
	c     *net.UDPConn
	hello []byte
	mu    sync.Mutex
	peer  netip.AddrPort
	seen  chan struct{} // closed when the plug-in first says hello
	once  sync.Once
}

func openLoopDev(port int, token string) (*loopDev, error) {
	if port <= 0 || port > 65535 || len(token) < 16 {
		return nil, errors.New("Windows VPN: port or token missing in the settings")
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return nil, fmt.Errorf("Windows VPN: cannot listen on 127.0.0.1:%d: %v", port, err)
	}
	_ = c.SetReadBuffer(8 << 20)
	_ = c.SetWriteBuffer(8 << 20)
	return &loopDev{c: c, hello: []byte(vpnHelloPrefix + token), seen: make(chan struct{})}, nil
}

func (d *loopDev) Name() string { return "NetMerge (Windows VPN)" }

func (d *loopDev) Read(b []byte) (int, error) {
	for {
		n, from, err := d.c.ReadFromUDPAddrPort(b)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if b[0] == 0 { // a message from the plug-in, not a packet
			if n == len(d.hello) && subtle.ConstantTimeCompare(b[:n], d.hello) == 1 {
				d.mu.Lock()
				d.peer = from
				d.mu.Unlock()
				d.once.Do(func() { close(d.seen) })
			}
			continue
		}
		d.mu.Lock()
		ok := from == d.peer
		d.mu.Unlock()
		if ok {
			return n, nil
		}
	}
}

func (d *loopDev) Write(p []byte) error {
	d.mu.Lock()
	to := d.peer
	d.mu.Unlock()
	if !to.IsValid() {
		return nil // the plug-in has not said hello yet: nowhere to send
	}
	_, err := d.c.WriteToUDPAddrPort(p, to)
	return err
}

func (d *loopDev) Close() error { return d.c.Close() }

// startVPNTun listens for the plug-in and finishes setting up once Windows has connected the VPN (that is when the
// address 198.18.0.1 exists and the listeners can use it). NetMerge asks Windows to connect right after this.
func (e *Engine) startVPNTun(c *Config) error {
	dev, err := openLoopDev(c.Tun.VPNPort, c.Tun.VPNToken)
	if err != nil {
		return err
	}
	mtu := tunMTU(c) // the plug-in asks Windows for the same size (the profile's <mtu>)
	t := &tunStack{e: e, dev: dev, mtu: mtu, nat: newNATTable(), done: make(chan struct{})}
	t.hijack.Store(c.DNS.Enable && len(c.Tun.DNSHijack) > 0)
	e.tun = t
	e.log.Infof("[TUN] Windows VPN (test): waiting for the plug-in on 127.0.0.1:%d", c.Tun.VPNPort)
	go t.finishVPN(c)
	return nil
}

func (t *tunStack) finishVPN(c *Config) {
	began := time.Now()
	d := t.dev.(*loopDev)
	// packets are read from the start, so the plug-in's hello is noticed; until the listeners exist they are dropped
	go t.readLoop()
	select {
	case <-d.seen:
	case <-t.done:
		return
	case <-time.After(90 * time.Second):
		t.e.log.Warnf("[TUN] Windows VPN (test): the plug-in did not answer in 90 s; is the \"NetMerge\" VPN connected in Windows settings?")
		select {
		case <-d.seen:
		case <-t.done:
			return
		}
	}
	t.e.log.Infof("[TUN] Windows VPN (test): plug-in connected after %d ms", time.Since(began).Milliseconds())
	var l4 *net.TCPListener
	for {
		var err error
		if l4, err = net.ListenTCP("tcp4", &net.TCPAddr{IP: tunGW4.AsSlice(), Port: 0}); err == nil {
			break
		}
		select {
		case <-t.done:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if t.closed.Load() {
		l4.Close()
		return
	}
	if err := t.setupOS(c); err != nil {
		t.e.log.Warnf("[TUN] Windows VPN (test): %v", err)
	}
	t.ln4 = l4
	t.port4.Store(uint32(l4.Addr().(*net.TCPAddr).Port))
	if t.closed.Load() { // stopped while setting up
		l4.Close()
		return
	}
	go t.accept(l4)
	go t.janitor()
	t.e.log.Infof("[TUN] capturing traffic through the Windows VPN (MTU %d), ready in %d ms", t.mtu, time.Since(began).Milliseconds())
}

// waitAddressGone waits until no interface has addr (the Windows VPN's 198.18.0.1 after it was disconnected), so
// NetMerge's own adapter does not get an address that is still in use: Windows then leaves it unusable.
func waitAddressGone(addr netip.Addr, limit time.Duration, e *Engine) {
	began := time.Now()
	for hasAddress(addr) {
		if time.Since(began) > limit {
			e.log.Warnf("[TUN] %s is still in use by another network adapter after %s (is the Windows VPN \"NetMerge\" still connected?)", addr, limit)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	if d := time.Since(began); d > 300*time.Millisecond {
		e.log.Infof("[TUN] waited %d ms for the Windows VPN to let go of %s", d.Milliseconds(), addr)
	}
}

func hasAddress(addr netip.Addr) bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, ifc := range ifs {
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if p, ok := a.(*net.IPNet); ok && p.IP.Equal(addr.AsSlice()) {
				return true
			}
		}
	}
	return false
}
