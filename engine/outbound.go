package engine

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Outbound is where a connection can be sent: a line (one network adapter), DIRECT, REJECT, or a group of them.
type Outbound interface {
	Name() string
	Type() string // as /proxies reports it: Direct, Reject, Selector, LoadBalance
}

// Line sends traffic out through one network adapter. Several Line values can share an adapter (weights: "LINK1#2").
type Line struct {
	name  string
	iface string // adapter name; "" = the system's normal route (DIRECT)
	udp   bool
	// failures counts recent connect failures, so a broken line is passed over for a short while
	failUntil atomic.Int64
}

func (l *Line) Name() string { return l.name }
func (l *Line) Type() string { return "Direct" }

// Base is the adapter-level identity: "LINK1#2" and "LINK1" are the same line.
func (l *Line) Base() string {
	if i := strings.IndexByte(l.name, '#'); i > 0 {
		return l.name[:i]
	}
	return l.name
}

func (l *Line) healthy() bool { return time.Now().UnixNano() >= l.failUntil.Load() }
func (l *Line) markFailed()   { l.failUntil.Store(time.Now().Add(15 * time.Second).UnixNano()) }
func (l *Line) markOK()       { l.failUntil.Store(0) }

type Reject struct{}

func (Reject) Name() string { return "REJECT" }
func (Reject) Type() string { return "Reject" }

// Selector points at one member; NetMerge changes it through the API (SLOTn, CALL, ONLY).
type Selector struct {
	name    string
	members []string
	mu      sync.RWMutex
	now     string
}

func (s *Selector) Name() string { return s.name }
func (s *Selector) Type() string { return "Selector" }
func (s *Selector) Now() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.now
}
func (s *Selector) Set(name string) bool {
	for _, m := range s.members {
		if m == name {
			s.mu.Lock()
			s.now = name
			s.mu.Unlock()
			return true
		}
	}
	return false
}

// LoadBalance spreads connections over its members: in turn (round-robin) or by website (consistent-hashing).
type LoadBalance struct {
	name     string
	strategy string
	members  []string
	next     atomic.Uint64
}

func (b *LoadBalance) Name() string { return b.name }
func (b *LoadBalance) Type() string { return "LoadBalance" }

// Router knows every outbound and turns "a group" into an ordered list of lines to try.
type Router struct {
	outs  map[string]Outbound
	order []string // names in config order, for /proxies
}

func newRouter(c *Config) (*Router, error) {
	r := &Router{outs: map[string]Outbound{}}
	add := func(o Outbound) {
		r.outs[o.Name()] = o
		r.order = append(r.order, o.Name())
	}
	add(&Line{name: "DIRECT", udp: true})
	add(Reject{})
	for _, p := range c.Proxies {
		if p.Type != "direct" {
			return nil, fmt.Errorf("line %s: type %q is not supported", p.Name, p.Type)
		}
		add(&Line{name: p.Name, iface: p.InterfaceName, udp: p.UDP})
	}
	for _, g := range c.Groups {
		switch g.Type {
		case "select":
			s := &Selector{name: g.Name, members: g.Proxies}
			if len(g.Proxies) > 0 {
				s.now = g.Proxies[0]
			}
			add(s)
		case "load-balance":
			st := g.Strategy
			if st == "" {
				st = "consistent-hashing"
			}
			add(&LoadBalance{name: g.Name, strategy: st, members: g.Proxies})
		default:
			return nil, fmt.Errorf("group %s: type %q is not supported", g.Name, g.Type)
		}
	}
	for _, g := range c.Groups {
		for _, m := range g.Proxies {
			if _, ok := r.outs[m]; !ok {
				return nil, fmt.Errorf("group %s lists %s, which does not exist", g.Name, m)
			}
		}
	}
	return r, nil
}

// carry over what NetMerge chose (SLOTn, CALL, ONLY) when the settings are reloaded, as far as it still exists
func (r *Router) keepChoices(old *Router) {
	if old == nil {
		return
	}
	for n, o := range r.outs {
		if s, ok := o.(*Selector); ok {
			if os, ok := old.outs[n].(*Selector); ok {
				s.Set(os.Now())
			}
		}
	}
}

// plan is the order of lines to try for one connection, and the chain of names for the connection list.
type plan struct {
	lines  []*Line
	chain  []string // first entry: the line actually used (filled in after connecting), then the groups
	reject bool
}

// resolvePlan expands target into lines: the chosen one first, then the other members of the groups on the way
// as fallbacks, in case the chosen line cannot connect.
func (r *Router) resolvePlan(target string, m *Metadata) (*plan, error) {
	p := &plan{}
	var groups []string // the groups on the chosen path, outermost first
	var walk func(name string, depth int, primary bool) ([]*Line, error)
	walk = func(name string, depth int, primary bool) ([]*Line, error) {
		if depth > 8 {
			return nil, errors.New("groups refer to each other in a loop")
		}
		o, ok := r.outs[name]
		if !ok {
			return nil, fmt.Errorf("unknown outbound %s", name)
		}
		switch x := o.(type) {
		case Reject:
			if primary {
				p.reject = true
			}
			return nil, nil
		case *Line:
			return []*Line{x}, nil
		case *Selector:
			if primary {
				groups = append(groups, x.name)
			}
			now := x.Now()
			out, err := walk(now, depth+1, primary)
			if err != nil || p.reject {
				return out, err
			}
			for _, mn := range x.members { // NetMerge's explicit choice first; the rest only as a fallback
				if mn != now {
					more, _ := walk(mn, depth+1, false)
					out = append(out, more...)
				}
			}
			return out, nil
		case *LoadBalance:
			if len(x.members) == 0 {
				return nil, fmt.Errorf("group %s is empty", x.name)
			}
			if primary {
				groups = append(groups, x.name)
			}
			start := 0
			if x.strategy == "round-robin" {
				if primary {
					start = int((x.next.Add(1) - 1) % uint64(len(x.members)))
				}
			} else {
				start = int(hashKey(m) % uint64(len(x.members)))
			}
			var out []*Line
			for i := 0; i < len(x.members); i++ {
				more, err := walk(x.members[(start+i)%len(x.members)], depth+1, primary && i == 0)
				if err != nil {
					return nil, err
				}
				if p.reject {
					return nil, nil
				}
				out = append(out, more...)
			}
			return out, nil
		}
		return nil, fmt.Errorf("unknown outbound %s", name)
	}
	lines, err := walk(target, 0, true)
	if err != nil {
		return nil, err
	}
	// the same adapter listed twice (weights): keep the first; lines that just failed go to the back
	seen := map[string]bool{}
	var good, later []*Line
	for _, l := range lines {
		k := l.Base() + "|" + l.iface
		if seen[k] {
			continue
		}
		seen[k] = true
		if l.healthy() {
			good = append(good, l)
		} else {
			later = append(later, l)
		}
	}
	p.lines = append(good, later...)
	for i := len(groups) - 1; i >= 0; i-- { // mihomo's order: line, then groups from the innermost out
		p.chain = append(p.chain, groups[i])
	}
	return p, nil
}

// hashKey keeps one website on one line: the registrable part of its name (or the address).
func hashKey(m *Metadata) uint64 {
	k := m.ruleHost()
	if k != "" {
		parts := strings.Split(strings.ToLower(k), ".")
		if len(parts) > 2 {
			k = strings.Join(parts[len(parts)-2:], ".")
			if len(parts[len(parts)-2]) <= 3 && len(parts) > 2 { // co.uk, com.pk ...
				k = strings.Join(parts[len(parts)-3:], ".")
			}
		}
	} else if m.DstIP.IsValid() {
		k = m.DstIP.Unmap().String()
	}
	h := fnv.New64a()
	h.Write([]byte(k))
	return h.Sum64()
}

// ---- dialing ----

// dialTimeout is per line: a line that cannot connect in this time is skipped for the next one.
const dialTimeout = 5 * time.Second

type dialer struct {
	e *Engine
}

// dialTCP connects to the destination through the plan's lines. The first line starts at once; if it has not
// connected within lineHeadStart, the next one starts too, and so on - the first to connect wins, the rest are
// cancelled. A line that fails is passed over for a short while, so the next connections do not wait for it.
const lineHeadStart = 1200 * time.Millisecond

func (d *dialer) dialTCP(ctx context.Context, p *plan, m *Metadata) (net.Conn, *Line, error) {
	if len(p.lines) == 0 {
		return nil, nil, errors.New("no line to use")
	}
	type result struct {
		c   net.Conn
		l   *Line
		err error
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan result, len(p.lines))
	started, pending := 0, 0
	start := func() {
		l := p.lines[started]
		started++
		pending++
		go func() {
			c, err := d.dialLine(ctx, l, "tcp", m)
			results <- result{c, l, err}
		}()
	}
	start()
	timer := time.NewTimer(lineHeadStart)
	defer timer.Stop()
	var lastErr error
	for pending > 0 {
		select {
		case r := <-results:
			pending--
			if r.err == nil {
				r.l.markOK()
				if r.l != p.lines[0] {
					d.e.log.Infof("[TCP] %s: %s did not connect in time, used %s instead", m.RemoteAddress(), p.lines[0].name, r.l.name)
					// the lines that started earlier and are still trying are too slow right now: pass them over
					for _, l := range p.lines {
						if l == r.l {
							break
						}
						l.markFailed()
					}
				}
				cancel()
				// close any later winner
				go func(n int) {
					for i := 0; i < n; i++ {
						if x := <-results; x.c != nil {
							x.c.Close()
						}
					}
				}(pending)
				return r.c, r.l, nil
			}
			lastErr = r.err
			d.e.log.Debugf("[TCP] %s via %s: %v", m.RemoteAddress(), r.l.name, r.err)
			if isDestinationError(r.err) {
				// the website itself said no (the line works): another line would hear the same answer
				r.l.markOK()
				cancel()
				go func(n int) {
					for i := 0; i < n; i++ {
						if x := <-results; x.c != nil {
							x.c.Close()
						}
					}
				}(pending)
				return nil, nil, r.err
			}
			if ctx.Err() == nil {
				r.l.markFailed()
			}
			if started < len(p.lines) {
				start()
				timer.Reset(lineHeadStart)
			}
		case <-timer.C:
			if started < len(p.lines) {
				start()
				timer.Reset(lineHeadStart)
			}
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}
	if lastErr == nil {
		lastErr = errors.New("no line could connect")
	}
	return nil, nil, lastErr
}

// isDestinationError: the far end answered "no" (refused / reset), so the line itself works.
func isDestinationError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "refused") || strings.Contains(s, "reset by peer")
}

func (d *dialer) dialLine(ctx context.Context, l *Line, network string, m *Metadata) (net.Conn, error) {
	addrs, err := d.addressesFor(ctx, l, m)
	if err != nil {
		return nil, err
	}
	iface := l.iface
	if iface == "" {
		iface = d.e.defaultInterface()
	}
	var lastErr error
	for _, a := range addrs {
		nd := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}
		if err := bindDialer(nd, iface, a); err != nil {
			return nil, err
		}
		c, err := nd.DialContext(ctx, network, netip.AddrPortFrom(a, m.DstPort).String())
		if err == nil {
			return c, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%s has no address", m.ruleHost())
	}
	return nil, lastErr
}

// addressesFor finds the addresses to connect to. A known address is used as it is; a name is looked up. IPv4
// comes first unless the engine runs with IPv6.
func (d *dialer) addressesFor(ctx context.Context, l *Line, m *Metadata) ([]netip.Addr, error) {
	host := m.Host
	if host == "" {
		if !m.DstIP.IsValid() {
			return nil, errors.New("no destination")
		}
		h, ok := d.e.fakeHost(m.DstIP)
		if !ok {
			return []netip.Addr{m.DstIP.Unmap()}, nil
		}
		host = h
	}
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a.Unmap()}, nil
	}
	return d.e.lookup(ctx, host, l)
}

// dialUDP opens a UDP socket on the first line of the plan that can send.
func (d *dialer) listenUDP(p *plan) (*net.UDPConn, *Line, error) {
	var lastErr error
	for _, l := range p.lines {
		if !l.udp {
			continue
		}
		iface := l.iface
		if iface == "" {
			iface = d.e.defaultInterface()
		}
		c, err := listenUDPOn(iface)
		if err == nil {
			return c, l, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no line carries UDP")
	}
	return nil, nil, lastErr
}
