package engine

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Capturing the PC's traffic. NetMerge's virtual adapter receives everything the PC sends to the internet. TCP is
// handed to the PC's own TCP/IP stack: each packet's addresses are rewritten so the connection arrives at a local
// listener of the engine (and replies are rewritten back), which is fast because Windows does the TCP work. UDP and
// DNS are read straight from the packets.

// tunDevice is the virtual adapter: one packet per Read/Write.
type tunDevice interface {
	Read(b []byte) (int, error)
	Write(p []byte) error
	Close() error
	Name() string
}

var (
	tunGW4     = netip.MustParseAddr("198.18.0.1") // the adapter's own address (apps' connections come from it)
	tunPortal4 = netip.MustParseAddr("198.18.0.2") // the far side used for rewritten packets
	tunNet4    = netip.MustParsePrefix("198.18.0.0/16")
	tunGW6     = netip.MustParseAddr("fdfe:dcba:9876::1")
	tunPortal6 = netip.MustParseAddr("fdfe:dcba:9876::2")
	tunNet6    = netip.MustParsePrefix("fdfe:dcba:9876::/126")
	tunDNS4    = tunPortal4 // the DNS address given to the adapter when the built-in DNS is on
)

type tunStack struct {
	e       *Engine
	dev     tunDevice
	mtu     int
	ln4     *net.TCPListener
	ln6     *net.TCPListener
	port4   uint16
	port6   uint16
	nat     *natTable
	wmu     sync.Mutex
	hijack  atomic.Bool // answer DNS (port 53) here
	osState any         // routes and settings to undo (per operating system)
	closed  atomic.Bool
	done    chan struct{}
}

func (e *Engine) tunPrefix() netip.Prefix {
	if t, ok := e.tun.(*tunStack); ok && t != nil {
		return tunNet4
	}
	return netip.Prefix{}
}

// applyTun starts capturing when the settings ask for it, and stops when they no longer do.
func (e *Engine) applyTun(c *Config) error {
	if !c.Tun.Enable {
		e.stopTun()
		return nil
	}
	if t, ok := e.tun.(*tunStack); ok && t != nil {
		t.hijack.Store(c.DNS.Enable && len(c.Tun.DNSHijack) > 0)
		t.updateExcludes(c.Tun.RouteExcludeAddress)
		return nil
	}
	mtu := c.Tun.MTU
	if mtu < 1280 || mtu > 65000 {
		mtu = 9000
	}
	dev, err := openTun("NetMerge", mtu)
	if err != nil {
		return err
	}
	t := &tunStack{e: e, dev: dev, mtu: mtu, nat: newNATTable(), done: make(chan struct{})}
	t.hijack.Store(c.DNS.Enable && len(c.Tun.DNSHijack) > 0)
	if err := t.setupOS(c); err != nil {
		dev.Close()
		return err
	}
	// listeners on the adapter's own addresses receive the rewritten TCP connections
	l4, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: tunGW4.AsSlice(), Port: 0})
	if err != nil {
		t.teardownOS()
		dev.Close()
		return err
	}
	t.ln4, t.port4 = l4, uint16(l4.Addr().(*net.TCPAddr).Port)
	if l6, err := net.ListenTCP("tcp6", &net.TCPAddr{IP: tunGW6.AsSlice(), Port: 0}); err == nil {
		t.ln6, t.port6 = l6, uint16(l6.Addr().(*net.TCPAddr).Port)
	} else {
		e.log.Warnf("tun: IPv6 listener: %v (IPv6 is not captured)", err)
	}
	e.tun = t
	go t.readLoop()
	go t.accept(t.ln4)
	if t.ln6 != nil {
		go t.accept(t.ln6)
	}
	go t.janitor()
	e.log.Infof("[TUN] capturing traffic on %s (MTU %d)", dev.Name(), mtu)
	return nil
}

func (e *Engine) stopTun() {
	t, ok := e.tun.(*tunStack)
	if !ok || t == nil {
		return
	}
	e.tun = nil
	t.close()
}

func (t *tunStack) close() {
	if t.closed.Swap(true) {
		return
	}
	close(t.done)
	if t.ln4 != nil {
		t.ln4.Close()
	}
	if t.ln6 != nil {
		t.ln6.Close()
	}
	t.teardownOS()
	t.dev.Close()
	t.e.log.Infof("[TUN] stopped")
}

func (t *tunStack) write(p []byte) {
	t.wmu.Lock()
	_ = t.dev.Write(p)
	t.wmu.Unlock()
}

// ---- the packet loop ----

func (t *tunStack) readLoop() {
	buf := make([]byte, 65535)
	for {
		n, err := t.dev.Read(buf)
		if err != nil {
			if t.closed.Load() || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		p, ok := parseIP(buf[:n])
		if !ok {
			t.e.log.Debugf("[TUN] unreadable packet (%d bytes)", n)
			continue
		}
		switch p.proto {
		case protoTCP:
			t.tcpPacket(p)
		case protoUDP:
			if !p.frag {
				t.udpPacket(p)
			}
		case protoICMP, protoICMPv6:
			// answered here, as mihomo does: the internet's reachability is better judged by real connections
			if echoReply(p) {
				t.write(p.b)
			}
		}
	}
}

func (t *tunStack) gw(v6 bool) (netip.Addr, netip.Addr, uint16) {
	if v6 {
		return tunGW6, tunPortal6, t.port6
	}
	return tunGW4, tunPortal4, t.port4
}

// tcpPacket rewrites one TCP packet: an app's packet goes to the engine's listener, the listener's reply goes back
// to the app as if it came from the website.
func (t *tunStack) tcpPacket(p ipPacket) {
	if p.frag || len(p.payload) < 20 {
		return
	}
	sp, dp, _ := l4Ports(p)
	gw, portal, lport := t.gw(p.v6)
	if lport == 0 {
		return
	}
	if p.src == gw && sp == lport {
		// the engine's listener answering: back to the app
		en := t.nat.byPort(dp)
		if en == nil {
			return
		}
		en.touch()
		setAddrs(&p, en.dst, en.app)
		t.write(p.b)
		return
	}
	if p.dst == gw || p.dst == portal {
		return
	}
	app, dst := netip.AddrPortFrom(p.src, sp), netip.AddrPortFrom(p.dst, dp)
	syn := p.payload[13]&0x02 != 0 && p.payload[13]&0x10 == 0
	en := t.nat.lookup(app, dst, syn)
	if en == nil {
		return // a stray packet of a connection the engine does not know
	}
	en.touch()
	setAddrs(&p, netip.AddrPortFrom(portal, en.port), netip.AddrPortFrom(gw, lport))
	t.write(p.b)
}

// accept takes the rewritten connections and hands them to the engine.
func (t *tunStack) accept(l *net.TCPListener) {
	for {
		c, err := l.AcceptTCP()
		if err != nil {
			if t.closed.Load() {
				return
			}
			time.Sleep(20 * time.Millisecond)
			continue
		}
		ra := c.RemoteAddr().(*net.TCPAddr)
		en := t.nat.byPort(uint16(ra.Port))
		if en == nil {
			c.Close()
			continue
		}
		go t.serveTCP(c, en)
	}
}

func (t *tunStack) serveTCP(c *net.TCPConn, en *natEntry) {
	defer t.nat.release(en)
	_ = c.SetNoDelay(true)
	m := &Metadata{Type: "Tun", SrcIP: en.app.Addr(), SrcPort: en.app.Port(), DstIP: en.dst.Addr().Unmap(), DstPort: en.dst.Port()}
	if h, ok := t.e.fakeHost(m.DstIP); ok {
		m.Host = h
	}
	if m.DstPort == 53 && t.hijack.Load() {
		t.dnsOverTCP(c)
		return
	}
	t.e.handleTCP(c, m)
}

// dnsOverTCP answers DNS asked over TCP (2-byte length, then the message).
func (t *tunStack) dnsOverTCP(c net.Conn) {
	defer c.Close()
	d := t.e.dns
	if d == nil {
		return
	}
	for {
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		var lb [2]byte
		if _, err := io.ReadFull(c, lb[:]); err != nil {
			return
		}
		q := make([]byte, binary.BigEndian.Uint16(lb[:]))
		if _, err := io.ReadFull(c, q); err != nil {
			return
		}
		r := d.answer(q)
		if r == nil {
			return
		}
		out := binary.BigEndian.AppendUint16(nil, uint16(len(r)))
		if _, err := c.Write(append(out, r...)); err != nil {
			return
		}
	}
}

// udpPacket passes one UDP packet on (or answers DNS here).
func (t *tunStack) udpPacket(p ipPacket) {
	if len(p.payload) < 8 {
		return
	}
	sp, dp, _ := l4Ports(p)
	data := p.payload[8:]
	if ul := int(binary.BigEndian.Uint16(p.payload[4:6])); ul >= 8 && ul-8 < len(data) {
		data = data[:ul-8]
	}
	app, dst := netip.AddrPortFrom(p.src, sp), netip.AddrPortFrom(p.dst, dp)
	if dp == 53 && t.hijack.Load() && t.e.dns != nil {
		q := append([]byte(nil), data...)
		go func() {
			if r := t.e.dns.answer(q); r != nil {
				t.write(buildUDP(dst, app, r))
			}
		}()
		return
	}
	// no broadcast or multicast through the lines
	if a := p.dst.Unmap(); a.IsMulticast() || (a.Is4() && a.As4()[3] == 255) || a.IsUnspecified() {
		return
	}
	m := &Metadata{Type: "Tun", Network: "udp", SrcIP: p.src.Unmap(), SrcPort: sp, DstIP: p.dst.Unmap(), DstPort: dp}
	t.e.udp.send(m, data, "tun|"+app.String(), func(b []byte, from netip.AddrPort, _ string) error {
		if from.Addr().Is4() != app.Addr().Is4() {
			from = dst // the answer came over the other IP version: present it as from the asked address
		}
		t.write(buildUDP(from, app, b))
		return nil
	})
}

// ---- the address translation table ----

type natEntry struct {
	port uint16         // the port on the portal side (what the listener sees)
	app  netip.AddrPort // the app's own address and port
	dst  netip.AddrPort // where the app wanted to go
	last atomic.Int64
	open atomic.Int32 // 1 while the engine serves the connection
}

func (n *natEntry) touch() { n.last.Store(time.Now().UnixNano()) }

type natKey struct{ app, dst netip.AddrPort }

type natTable struct {
	mu    sync.Mutex
	ports map[uint16]*natEntry
	keys  map[natKey]*natEntry
	next  uint16
}

func newNATTable() *natTable {
	return &natTable{ports: map[uint16]*natEntry{}, keys: map[natKey]*natEntry{}, next: 10000}
}

func (t *natTable) byPort(p uint16) *natEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.ports[p]
}

// lookup finds the entry for an app's connection; a new connection (SYN) gets a new port.
func (t *natTable) lookup(app, dst netip.AddrPort, syn bool) *natEntry {
	k := natKey{app, dst}
	t.mu.Lock()
	defer t.mu.Unlock()
	if en, ok := t.keys[k]; ok {
		return en
	}
	if !syn {
		return nil
	}
	for i := 0; i < 55000; i++ {
		p := t.next
		t.next++
		if t.next < 10000 || t.next > 64999 {
			t.next = 10000
		}
		if _, used := t.ports[p]; used {
			continue
		}
		en := &natEntry{port: p, app: app, dst: dst}
		en.touch()
		en.open.Store(1)
		t.ports[p] = en
		t.keys[k] = en
		return en
	}
	return nil
}

// release marks the connection as finished; the entry stays a little while for the last packets.
func (t *natTable) release(en *natEntry) {
	en.open.Store(0)
	en.touch()
}

func (t *natTable) sweep() {
	now := time.Now().UnixNano()
	t.mu.Lock()
	defer t.mu.Unlock()
	for p, en := range t.ports {
		idle := now - en.last.Load()
		if (en.open.Load() == 0 && idle > int64(60*time.Second)) || idle > int64(2*time.Hour) {
			delete(t.ports, p)
			delete(t.keys, natKey{en.app, en.dst})
		}
	}
}

func (t *tunStack) janitor() {
	tk := time.NewTicker(15 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-tk.C:
			t.nat.sweep()
			t.watchOS()
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
