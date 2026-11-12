// Package engine is NetMerge's own network engine: it takes connections (from the PC's traffic, or from phones
// through the sharing ports), decides by NetMerge's rules which line each one uses, and connects it there.
package engine

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Version is reported by GET /version.
var Version = "0.1.0"

// Engine is one running engine.
type Engine struct {
	dir     string
	log     *Logger
	tracker *Tracker
	dial    *dialer

	mu      sync.RWMutex
	cfg     *Config
	rules   []*Rule
	router  *Router
	sniff   sniffPorts
	authOK  map[string]bool // "user:pass"
	lanNets []netip.Prefix

	inbounds   map[string]io.Closer // by a key describing the listener; kept across reloads when unchanged
	api        *http_server
	tun        any            // *tunStack while capturing
	pendingTun chan tunOpened // the adapter being created in advance (first start)
	dns        *dnsService
	defIface   atomic.Value // string: the adapter for DIRECT while the PC's traffic is captured
	procCache  *processFinder
	stopOnce   sync.Once
	stopped    chan struct{}
	udp        *udpTable
}

type sniffPorts struct {
	enable bool
	http   portSet
	tls    portSet
	quic   portSet
	pureIP bool
}

// Run is the engine's main: "-d <folder>" (settings in <folder>/config.yaml), "-v" prints the version.
func Run(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("engine", flag.ContinueOnError)
	dir := fs.String("d", ".", "settings folder")
	ver := fs.Bool("v", false, "print the version")
	cfgFile := fs.String("f", "", "settings file (default <folder>/config.yaml)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *ver {
		fmt.Fprintf(stdout, "NetMerge engine %s\n", Version)
		return 0
	}
	abs, _ := filepath.Abs(*dir)
	e := New(abs, stdout)
	path := *cfgFile
	if path == "" {
		path = filepath.Join(abs, "config.yaml")
	}
	if err := e.Start(path); err != nil {
		e.log.Errorf("start failed: %v", err)
		fmt.Fprintf(os.Stderr, "start failed: %v\n", err)
		e.Stop()
		return 1
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-e.stopped:
	}
	e.Stop()
	return 0
}

func New(dir string, out io.Writer) *Engine {
	if out == nil {
		out = io.Discard
	}
	e := &Engine{dir: dir, log: NewLogger(out, "info"), tracker: newTracker(), inbounds: map[string]io.Closer{}, stopped: make(chan struct{})}
	e.dial = &dialer{e: e}
	e.procCache = newProcessFinder()
	e.udp = newUDPTable(e)
	e.defIface.Store("")
	return e
}

// Start loads the settings and opens everything they ask for.
func (e *Engine) Start(path string) error {
	if err := e.Load(path); err != nil {
		e.dropPendingTun()
		return err
	}
	e.log.Infof("NetMerge engine %s started", Version)
	return nil
}

// Load reads (or re-reads) the settings and applies them. Open connections keep running; listeners whose
// settings did not change are kept; NetMerge's choices (SLOTn, CALL) are carried over.
func (e *Engine) Load(path string) error {
	c, err := LoadConfig(path)
	if err != nil {
		return err
	}
	e.preopenTun(c)
	sets := map[string]*DomainSet{}
	for name, p := range c.RuleProviders {
		if p.Behavior != "" && p.Behavior != "domain" {
			return fmt.Errorf("rule set %s: only domain lists are supported", name)
		}
		full, err := resolvePath(e.dir, p.Path)
		if err != nil {
			return err
		}
		ds, err := LoadDomainSet(full)
		if err != nil {
			e.log.Warnf("rule set %s: %v (treated as empty)", name, err)
			ds = NewDomainSet()
		}
		sets[name] = ds
	}
	rules, err := ParseRules(c.Rules, sets)
	if err != nil {
		return err
	}
	router, err := newRouter(c)
	if err != nil {
		return err
	}
	for _, r := range rules {
		if _, ok := router.outs[r.Target]; !ok {
			return fmt.Errorf("a rule sends traffic to %s, which does not exist", r.Target)
		}
	}
	sp := sniffPorts{enable: c.Sniffer.Enable, pureIP: c.Sniffer.ParsePureIP}
	if c.Sniffer.Enable {
		for k, v := range c.Sniffer.Sniff {
			switch k {
			case "HTTP":
				sp.http = parsePorts(v.Ports)
			case "TLS":
				sp.tls = parsePorts(v.Ports)
			case "QUIC":
				sp.quic = parsePorts(v.Ports)
			}
		}
	}
	auth := map[string]bool{}
	for _, a := range c.Authentication {
		auth[a] = true
	}
	var lan []netip.Prefix
	for _, s := range c.LanAllowedIPs {
		if p, err := netip.ParsePrefix(s); err == nil {
			lan = append(lan, p)
		}
	}

	e.mu.Lock()
	router.keepChoices(e.router)
	e.cfg, e.rules, e.router, e.sniff, e.authOK, e.lanNets = c, rules, router, sp, auth, lan
	e.mu.Unlock()
	e.log.SetLevel(c.LogLevel)
	for n, s := range sets {
		e.log.Infof("rule set %s: %d entries", n, s.Len())
	}

	if err := e.applyListeners(c); err != nil {
		return err
	}
	if err := e.applyDNS(c); err != nil {
		return err
	}
	if err := e.applyTun(c); err != nil {
		return err
	}
	return nil
}

// Stop closes everything and restores the PC's network settings.
func (e *Engine) Stop() {
	e.stopOnce.Do(func() {
		e.mu.Lock()
		ins := e.inbounds
		e.inbounds = map[string]io.Closer{}
		e.mu.Unlock()
		for _, c := range ins {
			_ = c.Close()
		}
		e.dropPendingTun()
		e.stopTun()
		if e.dns != nil {
			e.dns.close()
		}
		e.tracker.closeAll()
		e.udp.closeAll()
		close(e.stopped)
	})
}

func (e *Engine) config() *Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

func (e *Engine) defaultInterface() string { return e.defIface.Load().(string) }

// ---- deciding ----

// decide runs the rules for m and returns the matching rule and the lines to try.
func (e *Engine) decide(m *Metadata) (*Rule, *plan, error) {
	e.mu.RLock()
	rules, router, mode := e.rules, e.router, e.cfg.Mode
	e.mu.RUnlock()
	env := &ruleEnv{process: e.findProcess, resolve: e.resolveForRule}
	if !m.processDone && e.config().FindProcessMode == "always" {
		e.findProcess(m) // NetMerge shows which program each connection belongs to
	}
	var hit *Rule
	if mode == "direct" {
		hit = &Rule{cond: matchAll{}, Target: "DIRECT"}
	} else {
		for _, r := range rules {
			if r.cond.match(m, env) {
				hit = r
				break
			}
		}
		if hit == nil {
			hit = &Rule{cond: matchAll{}, Target: "DIRECT"}
		}
	}
	p, err := router.resolvePlan(hit.Target, m)
	return hit, p, err
}

func (e *Engine) findProcess(m *Metadata) {
	m.processDone = true
	if e.config().FindProcessMode == "off" || !m.SrcIP.IsValid() || !e.isLocalSource(m.SrcIP) {
		return
	}
	m.Process, m.ProcessPath = e.procCache.find(m.Network, m.SrcIP, m.SrcPort)
}

// isLocalSource: the connection was started on this PC (not by a phone on the network).
func (e *Engine) isLocalSource(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() {
		return true
	}
	if t := e.tunPrefix(); t.IsValid() && t.Contains(ip) {
		return true
	}
	return false
}

func (e *Engine) resolveForRule(host string) (netip.Addr, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	as, err := e.lookup(ctx, host, nil)
	if err != nil || len(as) == 0 {
		return netip.Addr{}, false
	}
	return as[0], true
}

// lookup turns a name into addresses: through the built-in DNS when it is on, else the system's DNS.
func (e *Engine) lookup(ctx context.Context, host string, l *Line) ([]netip.Addr, error) {
	if d := e.dns; d != nil && d.enabled() {
		iface := ""
		if l != nil {
			iface = l.iface
		}
		return d.resolve(ctx, host, e.config().IPv6, iface)
	}
	network := "ip4"
	if e.config().IPv6 {
		network = "ip"
	}
	as, err := net.DefaultResolver.LookupNetIP(ctx, network, host)
	if err != nil {
		return nil, err
	}
	for i := range as {
		as[i] = as[i].Unmap()
	}
	return as, nil
}

// ---- handling a TCP connection (from any way in) ----

// traceOn (NM_TRACE=1) logs how long each step of a new connection took.
var traceOn = os.Getenv("NM_TRACE") != ""

// handleTCP decides, connects and relays one connection. conn is the side towards the app.
func (e *Engine) handleTCP(conn net.Conn, m *Metadata) { e.handleTCPReady(conn, m, nil) }

// handleTCPReady is handleTCP for ways in that must answer the app first (a proxy's "connected" or "failed"):
// ready is called once the outcome is known; returning false ends the connection.
func (e *Engine) handleTCPReady(conn net.Conn, m *Metadata, ready func(err error, rejected bool) bool) {
	m.Network = "tcp"
	t0 := time.Now()
	// the program's name is looked up while the app's first bytes (for the website's name) are awaited
	var proc chan [2]string
	if !m.processDone && e.config().FindProcessMode == "always" && m.SrcIP.IsValid() && e.isLocalSource(m.SrcIP) {
		proc = make(chan [2]string, 1)
		go func(n string, ip netip.Addr, port uint16) {
			a, b := e.procCache.find(n, ip, port)
			proc <- [2]string{a, b}
		}(m.Network, m.SrcIP, m.SrcPort)
	}
	conn = e.maybeSniff(conn, m)
	if proc != nil {
		r := <-proc
		m.Process, m.ProcessPath, m.processDone = r[0], r[1], true
	}
	t1 := time.Now()
	rule, p, err := e.decide(m)
	t2 := time.Now()
	if err != nil {
		e.log.Warnf("[TCP] %s --> %s: %v", srcText(m), m.RemoteAddress(), err)
		if ready != nil {
			ready(err, false)
		}
		conn.Close()
		return
	}
	if p.reject {
		e.log.Infof("[TCP] %s --> %s match %s using REJECT", srcText(m), m.RemoteAddress(), rule.Describe())
		if ready != nil {
			ready(nil, true)
		}
		conn.Close()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	out, used, err := e.dial.dialTCP(ctx, p, m)
	cancel()
	if traceOn && err == nil {
		accept := time.Duration(0)
		if !m.born.IsZero() {
			accept = t0.Sub(m.born)
		}
		e.log.Infof("[TRACE] tcp %s accept=%dus sniff+program=%dus decide=%dus dial=%dus", m.RemoteAddress(),
			accept.Microseconds(), t1.Sub(t0).Microseconds(), t2.Sub(t1).Microseconds(), time.Since(t2).Microseconds())
	}
	if err != nil {
		e.log.Warnf("[TCP] dial %s (match %s/%s) %s --> %s error: %v", usingText(p, nil), rule.Name(), rule.Payload(), srcText(m), m.RemoteAddress(), err)
		if ready != nil {
			ready(err, false)
		}
		conn.Close()
		return
	}
	if ready != nil && !ready(nil, false) {
		out.Close()
		conn.Close()
		return
	}
	chain := append([]string{used.Base()}, p.chain...)
	e.log.Infof("[TCP] %s --> %s match %s using %s", srcText(m), m.RemoteAddress(), rule.Describe(), usingText(p, used))
	tr := e.tracker.add(m, chain, rule, multiCloser{conn, out})
	defer e.tracker.remove(tr)
	e.relay(conn, out, tr)
}

type multiCloser []io.Closer

func (mc multiCloser) Close() error {
	for _, c := range mc {
		_ = c.Close()
	}
	return nil
}

// srcText is "1.2.3.4:5(app.exe)" or "1.2.3.4:5".
func srcText(m *Metadata) string {
	s := netip.AddrPortFrom(m.SrcIP.Unmap(), m.SrcPort).String()
	if m.Process != "" {
		s += "(" + m.Process + ")"
	}
	return s
}

// usingText is "AUTO[LINK1]" (outermost group and the line), or the line alone.
func usingText(p *plan, used *Line) string {
	line := "?"
	if used != nil {
		line = used.Base()
	} else if len(p.lines) > 0 {
		line = p.lines[0].Base()
	}
	if len(p.chain) == 0 {
		return line
	}
	return p.chain[len(p.chain)-1] + "[" + line + "]"
}

// ---- sniffing website names ----

// maybeSniff reads the first bytes the app sends (without consuming them) to learn the website name.
func (e *Engine) maybeSniff(conn net.Conn, m *Metadata) net.Conn {
	e.mu.RLock()
	sp := e.sniff
	e.mu.RUnlock()
	if !sp.enable || m.Host != "" || m.SniffHost != "" {
		return conn
	}
	isTLS, isHTTP := sp.tls.has(m.DstPort), sp.http.has(m.DstPort)
	if !isTLS && !isHTTP {
		return conn
	}
	if h, ok := e.fakeHost(m.DstIP); ok { // the built-in DNS already knows the name
		m.Host = h
		return conn
	}
	br := bufio.NewReaderSize(conn, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	// wait for the first piece of data, then look at whatever has arrived
	_, _ = br.Peek(1)
	buf, _ := br.Peek(br.Buffered())
	_ = conn.SetReadDeadline(time.Time{})
	if len(buf) > 0 {
		if isTLS {
			if h := sniffTLS(buf); h != "" {
				m.SniffHost = h
			}
		}
		if m.SniffHost == "" && isHTTP {
			if h := sniffHTTP(buf); h != "" {
				m.SniffHost = h
			}
		}
	}
	return &peekedConn{Conn: conn, r: br}
}

type peekedConn struct {
	net.Conn
	r *bufio.Reader
}

func (p *peekedConn) Read(b []byte) (int, error) { return p.r.Read(b) }

func (p *peekedConn) CloseWrite() error {
	if cw, ok := p.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return errors.New("not supported")
}
