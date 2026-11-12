package engine

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// The built-in DNS (only when NetMerge's custom DNS is on). Apps get a temporary address from the fake-ip range for
// each name; when they connect to it the engine knows the name and connects to the real address on the line the
// rules choose. Real lookups go to the chosen servers (plain or DNS-over-HTTPS), with a cache.

type dnsService struct {
	e        *Engine
	cfg      DNSConfig
	fake     *fakePool
	filter   *DomainSet
	system   *DomainSet // names that keep using the PC's own DNS (company names)
	mu       sync.Mutex
	cache    map[string]dnsCacheEntry
	clientMu sync.Mutex
	clients  map[string]*http.Client // DNS-over-HTTPS client per line ("" = the PC's default line)
	bootMu   sync.Mutex
	bootIPs  map[string][]netip.Addr
	sysSrv   []netip.Addr
}

type dnsCacheEntry struct {
	addrs []netip.Addr
	until time.Time
}

func (e *Engine) applyDNS(c *Config) error {
	if !c.DNS.Enable {
		if e.dns != nil {
			e.dns.close()
		}
		e.dns = nil
		return nil
	}
	if c.DNS.EnhancedMode != "" && c.DNS.EnhancedMode != "fake-ip" {
		return fmt.Errorf("dns: only fake-ip mode is supported")
	}
	rng := c.DNS.FakeIPRange
	if rng == "" {
		rng = "198.18.0.1/16"
	}
	pfx, err := netip.ParsePrefix(rng)
	if err != nil {
		return fmt.Errorf("dns: fake-ip-range: %w", err)
	}
	d := &dnsService{e: e, cfg: c.DNS, filter: NewDomainSet(), system: NewDomainSet(), cache: map[string]dnsCacheEntry{}, bootIPs: map[string][]netip.Addr{}}
	if e.dns != nil && e.dns.fake != nil && e.dns.fake.prefix == pfx.Masked() {
		d.fake = e.dns.fake // keep the addresses apps already have
	} else {
		d.fake = newFakePool(pfx)
	}
	for _, f := range c.DNS.FakeIPFilter {
		d.filter.Add(f)
	}
	for pat, servers := range c.DNS.NameserverPolicy {
		for _, s := range servers {
			if s == "system" {
				d.system.Add(pat)
				d.filter.Add(pat)
			}
		}
	}
	d.sysSrv = systemDNSServers()
	d.clients = map[string]*http.Client{}
	e.dns = d
	return nil
}

func (d *dnsService) enabled() bool { return d != nil }
func (d *dnsService) close()        {}

// ---- fake addresses ----

type fakePool struct {
	mu     sync.Mutex
	prefix netip.Prefix
	first  netip.Addr
	size   uint32
	next   uint32
	byHost map[string]netip.Addr
	byIP   map[netip.Addr]string
}

func newFakePool(p netip.Prefix) *fakePool {
	p = p.Masked()
	size := uint32(1) << (32 - p.Bits())
	if size > 1<<20 {
		size = 1 << 20
	}
	// skip the network address and the first two (NetMerge's virtual adapter and its DNS)
	first := p.Addr().Next().Next().Next()
	return &fakePool{prefix: p, first: first, size: size - 4, byHost: map[string]netip.Addr{}, byIP: map[netip.Addr]string{}}
}

func addrAdd(a netip.Addr, n uint32) netip.Addr {
	b := a.As4()
	v := binary.BigEndian.Uint32(b[:]) + n
	binary.BigEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

func (f *fakePool) get(host string) netip.Addr {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.byHost[host]; ok {
		return a
	}
	a := addrAdd(f.first, f.next%f.size)
	f.next++
	if old, ok := f.byIP[a]; ok { // the range went round: the oldest name gives its address up
		delete(f.byHost, old)
	}
	f.byHost[host] = a
	f.byIP[a] = host
	return a
}

func (f *fakePool) host(a netip.Addr) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, ok := f.byIP[a.Unmap()]
	return h, ok
}

func (e *Engine) isFakeIP(a netip.Addr) bool {
	d := e.dns
	return d != nil && a.IsValid() && d.fake.prefix.Contains(a.Unmap())
}

func (e *Engine) fakeHost(a netip.Addr) (string, bool) {
	d := e.dns
	if d == nil || !a.IsValid() {
		return "", false
	}
	return d.fake.host(a)
}

// ---- answering the PC's questions ----

// answer builds the reply to one DNS query message.
func (d *dnsService) answer(q []byte) []byte {
	name, qtype, qend, ok := parseQuestion(q)
	if !ok {
		return nil
	}
	if (qtype == 1 || qtype == 28) && !d.filter.Has(name) {
		if qtype == 28 && !d.e.config().IPv6 {
			return buildReply(q[:qend], nil, 0) // no IPv6: an empty answer, the app uses IPv4
		}
		if qtype == 1 {
			a := d.fake.get(name)
			return buildReply(q[:qend], []netip.Addr{a}, 1)
		}
	}
	// everything else goes to a real server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if r, err := d.forward(ctx, q, d.system.Has(name), ""); err == nil {
		return r
	}
	return buildServFail(q[:qend])
}

// ---- real lookups ----

// resolve looks host up through the line iface ("" = the PC's default line), so every line gets the answers
// meant for its own network.
func (d *dnsService) resolve(ctx context.Context, host string, v6 bool, iface string) ([]netip.Addr, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	d.mu.Lock()
	if c, ok := d.cache[host]; ok && time.Now().Before(c.until) {
		d.mu.Unlock()
		return c.addrs, nil
	}
	d.mu.Unlock()
	types := []uint16{1}
	if v6 {
		types = append(types, 28)
	}
	var all []netip.Addr
	ttl := uint32(600)
	var lastErr error
	for _, t := range types {
		q := buildQuery(host, t)
		r, err := d.forward(ctx, q, d.system.Has(host), iface)
		if err != nil {
			lastErr = err
			continue
		}
		as, minTTL := parseAnswers(r, t)
		all = append(all, as...)
		if minTTL > 0 && minTTL < ttl {
			ttl = minTTL
		}
	}
	if len(all) == 0 {
		if lastErr == nil {
			lastErr = fmt.Errorf("%s: no address", host)
		}
		return nil, lastErr
	}
	if ttl < 30 {
		ttl = 30
	}
	d.mu.Lock()
	if len(d.cache) > 10000 {
		d.cache = map[string]dnsCacheEntry{}
	}
	d.cache[host] = dnsCacheEntry{addrs: all, until: time.Now().Add(time.Duration(ttl) * time.Second)}
	d.mu.Unlock()
	return all, nil
}

// forward sends the raw query to the configured servers (or the PC's own for company names) and returns the first
// good answer. Servers are asked one after the other.
func (d *dnsService) forward(ctx context.Context, q []byte, system bool, iface string) ([]byte, error) {
	if system {
		iface = "" // company names: the PC's own line and servers
	}
	servers := d.cfg.Nameserver
	if system {
		servers = nil
		for _, a := range d.sysSrv {
			servers = append(servers, a.String())
		}
	}
	if len(servers) == 0 {
		servers = []string{"1.1.1.1", "8.8.8.8"}
	}
	var lastErr error
	for _, s := range servers {
		var r []byte
		var err error
		if strings.HasPrefix(s, "https://") {
			r, err = d.doh(ctx, s, q, iface)
		} else {
			r, err = d.udpQuery(ctx, s, q, iface)
		}
		if err == nil && len(r) >= 12 {
			return r, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no DNS server answered")
	}
	return nil, lastErr
}

// udpQuery asks a plain DNS server, through the PC's normal adapter (never through NetMerge's own).
func (d *dnsService) udpQuery(ctx context.Context, server string, q []byte, iface string) ([]byte, error) {
	if !strings.Contains(server, ":") || strings.Count(server, ":") > 1 && !strings.HasPrefix(server, "[") {
		server = net.JoinHostPort(strings.TrimPrefix(strings.TrimPrefix(server, "udp://"), "tcp://"), "53")
	}
	if iface == "" {
		iface = d.e.defaultInterface()
	}
	c, err := listenUDPOn(iface)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	ra, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, err
	}
	dl, ok := ctx.Deadline()
	if !ok || time.Until(dl) > 3*time.Second {
		dl = time.Now().Add(3 * time.Second)
	}
	_ = c.SetDeadline(dl)
	if _, err := c.WriteToUDP(q, ra); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	for {
		n, from, err := c.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}
		if from.IP.Equal(ra.IP) && n >= 12 && buf[0] == q[0] && buf[1] == q[1] {
			return append([]byte(nil), buf[:n]...), nil
		}
	}
}

// doh asks a DNS-over-HTTPS server (RFC 8484, POST).
func (d *dnsService) dohClient(iface string) *http.Client {
	d.clientMu.Lock()
	defer d.clientMu.Unlock()
	if c, ok := d.clients[iface]; ok {
		return c
	}
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.dialForDoH(ctx, network, addr, iface)
		},
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     90 * time.Second,
	}}
	d.clients[iface] = c
	return c
}

func (d *dnsService) doh(ctx context.Context, url string, q []byte, iface string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(q))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := d.dohClient(iface).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DNS-over-HTTPS answered %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64*1024))
}

// dialForDoH connects to a DoH server: a name is looked up with the plain "default-nameserver" addresses.
func (d *dnsService) dialForDoH(ctx context.Context, network, addr, iface string) (net.Conn, error) {
	if iface == "" {
		iface = d.e.defaultInterface()
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	var ips []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{a}
	} else {
		d.bootMu.Lock()
		ips = d.bootIPs[host]
		d.bootMu.Unlock()
		if len(ips) == 0 {
			boot := d.cfg.DefaultNameserver
			if len(boot) == 0 {
				boot = []string{"1.1.1.1", "8.8.8.8"}
			}
			for _, b := range boot {
				r, err := d.udpQuery(ctx, b, buildQuery(host, 1), iface)
				if err == nil {
					ips, _ = parseAnswers(r, 1)
					if len(ips) > 0 {
						break
					}
				}
			}
			d.bootMu.Lock()
			d.bootIPs[host] = ips
			d.bootMu.Unlock()
		}
	}
	var lastErr error
	for _, ip := range ips {
		nd := &net.Dialer{Timeout: 4 * time.Second}
		if err := bindDialer(nd, iface, ip); err != nil {
			return nil, err
		}
		c, err := nd.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return c, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("cannot find the DNS server %s", host)
	}
	return nil, lastErr
}

// ---- DNS messages (just what is needed) ----

func buildQuery(name string, qtype uint16) []byte {
	var id [2]byte
	binary.BigEndian.PutUint16(id[:], uint16(time.Now().UnixNano()))
	b := []byte{id[0], id[1], 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, qtype)
	return binary.BigEndian.AppendUint16(b, 1)
}

// parseQuestion returns the asked name and type, and where the question ends.
func parseQuestion(q []byte) (string, uint16, int, bool) {
	if len(q) < 12 || binary.BigEndian.Uint16(q[4:6]) != 1 {
		return "", 0, 0, false
	}
	var labels []string
	p := 12
	for {
		if p >= len(q) {
			return "", 0, 0, false
		}
		l := int(q[p])
		if l == 0 {
			p++
			break
		}
		if l&0xc0 != 0 || p+1+l > len(q) {
			return "", 0, 0, false
		}
		labels = append(labels, string(q[p+1:p+1+l]))
		p += 1 + l
	}
	if p+4 > len(q) {
		return "", 0, 0, false
	}
	return strings.ToLower(strings.Join(labels, ".")), binary.BigEndian.Uint16(q[p : p+2]), p + 4, true
}

// buildReply answers the question (header + question in qpart) with A or AAAA records (ttl 1 s for fake ones).
func buildReply(qpart []byte, addrs []netip.Addr, ttl uint32) []byte {
	r := append([]byte(nil), qpart...)
	r[2] = 0x81 | (qpart[2] & 0x01) // response, recursion desired as asked
	r[3] = 0x80                     // recursion available, no error
	binary.BigEndian.PutUint16(r[6:8], uint16(len(addrs)))
	binary.BigEndian.PutUint16(r[8:10], 0)
	binary.BigEndian.PutUint16(r[10:12], 0)
	for _, a := range addrs {
		r = append(r, 0xc0, 0x0c) // name: pointer to the question
		if a.Is4() {
			r = binary.BigEndian.AppendUint16(r, 1)
		} else {
			r = binary.BigEndian.AppendUint16(r, 28)
		}
		r = binary.BigEndian.AppendUint16(r, 1)
		r = binary.BigEndian.AppendUint32(r, ttl)
		s := a.AsSlice()
		r = binary.BigEndian.AppendUint16(r, uint16(len(s)))
		r = append(r, s...)
	}
	return r
}

func buildServFail(qpart []byte) []byte {
	r := append([]byte(nil), qpart...)
	r[2] = 0x81 | (qpart[2] & 0x01)
	r[3] = 0x82 // SERVFAIL
	binary.BigEndian.PutUint16(r[6:8], 0)
	return r
}

func skipName(b []byte, p int) int {
	for p < len(b) {
		l := int(b[p])
		switch {
		case l == 0:
			return p + 1
		case l&0xc0 == 0xc0:
			return p + 2
		default:
			p += 1 + l
		}
	}
	return -1
}

// parseAnswers returns the addresses of type qtype in a reply, and the smallest TTL among them.
func parseAnswers(r []byte, qtype uint16) ([]netip.Addr, uint32) {
	if len(r) < 12 || r[3]&0x0f != 0 {
		return nil, 0
	}
	qd, an := int(binary.BigEndian.Uint16(r[4:6])), int(binary.BigEndian.Uint16(r[6:8]))
	p := 12
	for i := 0; i < qd; i++ {
		if p = skipName(r, p); p < 0 || p+4 > len(r) {
			return nil, 0
		}
		p += 4
	}
	var out []netip.Addr
	var minTTL uint32
	for i := 0; i < an; i++ {
		if p = skipName(r, p); p < 0 || p+10 > len(r) {
			break
		}
		t := binary.BigEndian.Uint16(r[p:])
		ttl := binary.BigEndian.Uint32(r[p+4:])
		l := int(binary.BigEndian.Uint16(r[p+8:]))
		p += 10
		if p+l > len(r) {
			break
		}
		if t == qtype && (l == 4 || l == 16) {
			a, _ := netip.AddrFromSlice(r[p : p+l])
			out = append(out, a.Unmap())
			if minTTL == 0 || ttl < minTTL {
				minTTL = ttl
			}
		}
		p += l
	}
	return out, minTTL
}
