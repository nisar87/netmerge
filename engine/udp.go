package engine

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

// UDP: each sender (an app's socket, a phone) gets one outgoing socket per line, so replies from any server find
// their way back ("full cone", which calls and games need). The line is decided once per destination.

type replyFunc func(p []byte, src netip.AddrPort, host string) error

type udpTable struct {
	e        *Engine
	mu       sync.Mutex
	sessions map[string]*udpSession // client + line
	flows    map[string]*udpFlow    // client + destination
}

type udpSession struct {
	key   string
	conn  *net.UDPConn
	line  *Line
	reply replyFunc
	last  atomic.Int64
	mu    sync.Mutex
	flows map[netip.AddrPort]*udpFlow // by the real remote address
}

type udpFlow struct {
	pending bool     // being opened: packets wait in queue
	queue   [][]byte // at most a few packets while the line is chosen
	key     string
	sess    *udpSession
	remote  netip.AddrPort // where packets go
	orig    netip.AddrPort // the address the sender used (a built-in DNS address, say): replies come "from" it
	host    string         // the name it was asked for, if any (for replies)
	reject  bool
	tr      *tracked
	last    atomic.Int64
}

func newUDPTable(e *Engine) *udpTable {
	t := &udpTable{e: e, sessions: map[string]*udpSession{}, flows: map[string]*udpFlow{}}
	go t.janitor()
	return t
}

const udpIdle = 2 * time.Minute

func (t *udpTable) janitor() {
	tk := time.NewTicker(20 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-t.e.stopped:
			return
		case <-tk.C:
		}
		now := time.Now().UnixNano()
		t.mu.Lock()
		for k, f := range t.flows {
			idle := udpIdle
			if f.remote.Port() == 53 {
				idle = 20 * time.Second
			}
			if now-f.last.Load() > int64(idle) {
				delete(t.flows, k)
				if f.tr != nil {
					t.e.tracker.remove(f.tr)
				}
				if f.sess != nil {
					f.sess.mu.Lock()
					delete(f.sess.flows, f.remote)
					f.sess.mu.Unlock()
				}
			}
		}
		for k, s := range t.sessions {
			if now-s.last.Load() > int64(udpIdle) {
				delete(t.sessions, k)
				s.conn.Close()
			}
		}
		t.mu.Unlock()
	}
}

func (t *udpTable) closeAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range t.sessions {
		s.conn.Close()
	}
	t.sessions = map[string]*udpSession{}
	t.flows = map[string]*udpFlow{}
}

// send passes one packet from client towards m's destination. It never waits: the first packet of a new
// destination starts the decision in the background, and the next few packets wait in a short queue.
func (t *udpTable) send(m *Metadata, payload []byte, client string, reply replyFunc) {
	m.Network = "udp"
	fkey := client + ">" + m.RemoteAddress()
	t.mu.Lock()
	f := t.flows[fkey]
	if f == nil {
		f = &udpFlow{key: fkey, pending: true, queue: [][]byte{append([]byte(nil), payload...)}}
		f.last.Store(time.Now().UnixNano())
		t.flows[fkey] = f
		t.mu.Unlock()
		mc := *m
		go t.openFlow(f, &mc, client, reply)
		return
	}
	if f.pending {
		if len(f.queue) < 32 {
			f.queue = append(f.queue, append([]byte(nil), payload...))
		}
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	t.write(f, payload)
}

func (t *udpTable) write(f *udpFlow, payload []byte) {
	now := time.Now().UnixNano()
	f.last.Store(now)
	if f.reject || f.sess == nil {
		return
	}
	f.sess.last.Store(now)
	if n, err := f.sess.conn.WriteToUDPAddrPort(payload, f.remote); err == nil && f.tr != nil {
		t.e.tracker.addUp(f.tr, int64(n))
	}
}

// finish ends the "pending" state: on success the queued packets are sent, on failure the flow is forgotten so
// a later packet tries again.
func (t *udpTable) finish(f *udpFlow, ok bool) {
	t.mu.Lock()
	q := f.queue
	f.queue, f.pending = nil, false
	if !ok {
		delete(t.flows, f.key)
	}
	t.mu.Unlock()
	if ok {
		for _, p := range q {
			t.write(f, p)
		}
	}
}

func (t *udpTable) openFlow(f *udpFlow, m *Metadata, client string, reply replyFunc) {
	e := t.e
	var payload []byte
	if len(f.queue) > 0 {
		payload = f.queue[0]
	}
	// learn the name: from the built-in DNS, or from a QUIC packet
	if m.Host == "" {
		if h, ok := e.fakeHost(m.DstIP); ok {
			m.Host = h
		} else {
			e.mu.RLock()
			sp := e.sniff
			e.mu.RUnlock()
			if sp.enable && sp.quic.has(m.DstPort) {
				if h := sniffQUIC(payload); h != "" {
					m.SniffHost = h
				}
			}
		}
	}
	t0 := time.Now()
	rule, p, err := e.decide(m)
	t1 := time.Now()
	f.host = m.Host
	if err != nil {
		e.log.Warnf("[UDP] %s --> %s: %v", srcText(m), m.RemoteAddress(), err)
		t.finish(f, false)
		return
	}
	if p.reject {
		e.log.Infof("[UDP] %s --> %s match %s using REJECT", srcText(m), m.RemoteAddress(), rule.Describe())
		f.reject = true
		t.finish(f, true)
		return
	}
	// the address to send to
	var remote netip.AddrPort
	if m.Host != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		var first *Line
		if len(p.lines) > 0 {
			first = p.lines[0]
		}
		as, err := e.lookup(ctx, m.Host, first)
		cancel()
		if err != nil || len(as) == 0 {
			e.log.Warnf("[UDP] dial %s (match %s/%s) %s --> %s error: cannot find the address", usingText(p, nil), rule.Name(), rule.Payload(), srcText(m), m.RemoteAddress())
			t.finish(f, false)
			return
		}
		remote = netip.AddrPortFrom(as[0], m.DstPort)
	} else {
		remote = netip.AddrPortFrom(m.DstIP.Unmap(), m.DstPort)
	}
	f.remote = remote
	if m.DstIP.IsValid() {
		f.orig = netip.AddrPortFrom(m.DstIP.Unmap(), m.DstPort)
	}
	// one socket per client and line
	var sess *udpSession
	var used *Line
	for _, l := range p.lines {
		if !l.udp {
			continue
		}
		skey := client + "|" + l.Base()
		t.mu.Lock()
		sess = t.sessions[skey]
		t.mu.Unlock()
		if sess != nil {
			used = l
			break
		}
		iface := l.iface
		if iface == "" {
			iface = e.defaultInterface()
		}
		c, err := listenUDPOn(iface)
		if err != nil {
			e.log.Warnf("[UDP] %s: %v", l.name, err)
			continue
		}
		sess = &udpSession{key: skey, conn: c, line: l, reply: reply, flows: map[netip.AddrPort]*udpFlow{}}
		sess.last.Store(time.Now().UnixNano())
		t.mu.Lock()
		t.sessions[skey] = sess
		t.mu.Unlock()
		go t.readLoop(sess)
		used = l
		break
	}
	if sess == nil {
		e.log.Warnf("[UDP] dial %s (match %s/%s) %s --> %s error: no line carries UDP", usingText(p, nil), rule.Name(), rule.Payload(), srcText(m), m.RemoteAddress())
		t.finish(f, false)
		return
	}
	f.sess = sess
	e.log.Infof("[UDP] %s --> %s match %s using %s", srcText(m), m.RemoteAddress(), rule.Describe(), usingText(p, used))
	f.tr = e.tracker.add(m, append([]string{used.Base()}, p.chain...), rule, nil)
	sess.mu.Lock()
	sess.flows[remote] = f
	sess.mu.Unlock()
	t.finish(f, true)
	if traceOn {
		e.log.Infof("[TRACE] udp %s decide+program=%dus socket=%dus", m.RemoteAddress(), t1.Sub(t0).Microseconds(), time.Since(t1).Microseconds())
	}
}

func (t *udpTable) readLoop(s *udpSession) {
	buf := make([]byte, 65535)
	for {
		n, from, err := s.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		from = netip.AddrPortFrom(from.Addr().Unmap(), from.Port())
		s.last.Store(time.Now().UnixNano())
		s.mu.Lock()
		f := s.flows[from]
		s.mu.Unlock()
		host, src := "", from
		if f != nil {
			f.last.Store(time.Now().UnixNano())
			host = f.host
			if f.orig.IsValid() {
				src = f.orig
			}
			if f.tr != nil {
				t.e.tracker.addDown(f.tr, int64(n))
			}
		}
		_ = s.reply(buf[:n], src, host)
	}
}
