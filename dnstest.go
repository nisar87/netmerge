package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// DNS benchmark: real lookups sent out of each connection to every candidate server (plain and encrypted), timed,
// counted for failures and checked for an ISP that answers names that do not exist. Nothing here changes any setting.

var dnsNames = []string{"google.com", "youtube.com", "facebook.com", "wikipedia.org", "amazon.com", "cloudflare.com",
	"microsoft.com", "apple.com", "netflix.com", "github.com", "whatsapp.com", "instagram.com"}

type dnsTarget struct {
	ID, Label, Provider, Kind string // Kind: router | plain | doh | custom
	Addr, Boot                string // ip, or URL for doh ("" for router: each line's own gateway); Boot = plain server that finds a DoH host name
}

type DNSLineStat struct {
	Median float64 `json:"median"` // ms, -1 = no answer
	P90    float64 `json:"p90"`
	Fail   float64 `json:"fail"` // percent
	Hijack bool    `json:"hijack"`
	N      int     `json:"n"`
}

type DNSRow struct {
	ID       string                 `json:"id"`
	Label    string                 `json:"label"`
	Provider string                 `json:"provider"`
	Kind     string                 `json:"kind"`
	Lines    map[string]DNSLineStat `json:"lines"`
	Median   float64                `json:"median"` // average of the lines' medians, -1 = none
	Fail     float64                `json:"fail"`
	Hijack   bool                   `json:"hijack"`
}

type DNSRec struct {
	Provider string   `json:"provider"` // "" = nothing better than what you have
	Secure   bool     `json:"secure"`
	Median   float64  `json:"median"`
	Current  float64  `json:"current"`
	Text     string   `json:"text"`
	Notes    []string `json:"notes"`
}

type DNSTestState struct {
	Running  bool              `json:"running"`
	Progress int               `json:"progress"`
	Total    int               `json:"total"`
	Done     int64             `json:"done"` // unix seconds
	Lines    []string          `json:"lines"`
	LineName map[string]string `json:"lineNames"`
	Rows     []DNSRow          `json:"rows"`
	Rec      *DNSRec           `json:"rec,omitempty"`
	Error    string            `json:"error,omitempty"`
}

var (
	dnsTestMu sync.Mutex
	dnsTest   DNSTestState
)

func getDNSTest() DNSTestState {
	dnsTestMu.Lock()
	defer dnsTestMu.Unlock()
	t := dnsTest
	t.Rows = append([]DNSRow(nil), dnsTest.Rows...)
	return t
}

// ---- wire format (just enough: one A question, read the answer count and result code)

func dnsQuery(name string) (id uint16, pkt []byte) {
	var b [2]byte
	_, _ = rand.Read(b[:])
	id = binary.BigEndian.Uint16(b[:])
	pkt = []byte{byte(id >> 8), byte(id), 0x01, 0x00, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, l := range strings.Split(name, ".") {
		pkt = append(pkt, byte(len(l)))
		pkt = append(pkt, l...)
	}
	return id, append(pkt, 0, 0, 1, 0, 1)
}

// parseDNS returns the result code and the number of answers; ok is false for anything that is not our answer.
func parseDNS(id uint16, p []byte) (rcode, answers int, ok bool) {
	if len(p) < 12 || binary.BigEndian.Uint16(p) != id || p[2]&0x80 == 0 {
		return 0, 0, false
	}
	return int(p[3] & 0x0f), int(binary.BigEndian.Uint16(p[6:8])), true
}

func randName(suffix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("nm%x.%s", b, suffix)
}

func dialFor(a Adapter, timeout time.Duration) *net.Dialer {
	d := &net.Dialer{Timeout: timeout}
	d.Control = bindControl(a.Index)
	if ip := net.ParseIP(a.IP); ip != nil {
		d.LocalAddr = &net.UDPAddr{IP: ip}
	}
	return d
}

func queryUDP(a Adapter, server, name string) (ms float64, rcode, answers int, err error) {
	d := dialFor(a, 2*time.Second)
	c, err := d.Dial("udp4", net.JoinHostPort(server, "53"))
	if err != nil {
		// the dialer's local address is a TCP/UDP mix-up on some systems: try again without it
		d.LocalAddr = nil
		if c, err = d.Dial("udp4", net.JoinHostPort(server, "53")); err != nil {
			return 0, 0, 0, err
		}
	}
	defer c.Close()
	id, pkt := dnsQuery(name)
	_ = c.SetDeadline(time.Now().Add(2 * time.Second))
	start := time.Now()
	if _, err = c.Write(pkt); err != nil {
		return 0, 0, 0, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return 0, 0, 0, err
		}
		if rc, an, ok := parseDNS(id, buf[:n]); ok {
			return float64(time.Since(start).Microseconds()) / 1000, rc, an, nil
		}
	}
}

func dohClient(a Adapter, boot string) *http.Client {
	d := &net.Dialer{Timeout: 3 * time.Second, Control: bindControl(a.Index)}
	if boot != "" {
		// find the provider's host name through the provider's own plain address, out of this very line (the system
		// resolver may answer with a tunnel address instead)
		d.Resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			bd := &net.Dialer{Timeout: 2 * time.Second, Control: bindControl(a.Index)}
			return bd.DialContext(ctx, "udp4", net.JoinHostPort(boot, "53"))
		}}
	}
	if ip := net.ParseIP(a.IP); ip != nil {
		d.LocalAddr = &net.TCPAddr{IP: ip}
	}
	tr := &http.Transport{
		Proxy: nil, DialContext: d.DialContext, TLSClientConfig: &tls.Config{},
		TLSHandshakeTimeout: 4 * time.Second, ForceAttemptHTTP2: true, MaxIdleConns: 2, IdleConnTimeout: 20 * time.Second,
	}
	return &http.Client{Transport: tr, Timeout: 4 * time.Second}
}

func queryDoH(c *http.Client, url, name string) (ms float64, rcode, answers int, err error) {
	id, pkt := dnsQuery(name)
	req, _ := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(pkt))
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != 200 {
		return 0, 0, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	rc, an, ok := parseDNS(id, body)
	if !ok {
		return 0, 0, 0, fmt.Errorf("bad answer")
	}
	return float64(time.Since(start).Microseconds()) / 1000, rc, an, nil
}

// measure runs the question set against one server through one line.
func measureDNS(a Adapter, t dnsTarget) DNSLineStat {
	var q func(name string) (float64, int, int, error)
	if t.Kind == "doh" {
		c := dohClient(a, t.Boot)
		defer c.CloseIdleConnections()
		q = func(name string) (float64, int, int, error) { return queryDoH(c, t.Addr, name) }
	} else {
		addr := t.Addr
		if t.Kind == "router" {
			addr = a.Gateway
		}
		if net.ParseIP(addr) == nil {
			return DNSLineStat{Median: -1, Fail: 100}
		}
		q = func(name string) (float64, int, int, error) { return queryUDP(a, addr, name) }
	}
	_, _, _, _ = q("example.com") // warm-up: opens the connection / fills the server cache, not counted
	var times []float64
	fails, n := 0, 0
	for _, name := range dnsNames {
		n++
		ms, rc, an, err := q(name)
		if err != nil || rc != 0 || an == 0 {
			fails++
			continue
		}
		times = append(times, ms)
	}
	st := DNSLineStat{N: n, Fail: float64(fails) * 100 / float64(n), Median: -1, P90: -1}
	if len(times) > 0 {
		sort.Float64s(times)
		st.Median = times[len(times)/2]
		st.P90 = times[(len(times)*9)/10]
	}
	// a name that cannot exist must come back as "no such name", not as an address (some ISPs redirect it)
	for i := 0; i < 2; i++ {
		_, rc, an, err := q(randName("com"))
		if err == nil && rc == 0 && an > 0 {
			st.Hijack = true
		}
	}
	return st
}

func buildTargets() []dnsTarget {
	t := []dnsTarget{{ID: "router", Label: "Your current DNS (router / ISP)", Provider: "windows", Kind: "router"}}
	for _, p := range dnsProviders {
		t = append(t, dnsTarget{ID: p.ID + ":plain", Label: p.Name, Provider: p.ID, Kind: "plain", Addr: p.Plain[0]},
			dnsTarget{ID: p.ID + ":doh", Label: p.Name + " · encrypted", Provider: p.ID, Kind: "doh", Addr: p.DoH[0], Boot: bootOf(p)})
	}
	s := getSettings()
	if c := cleanIPs(s.DNSCustom); len(c) > 0 {
		t = append(t, dnsTarget{ID: "custom", Label: "Your own server (" + c[0] + ")", Provider: "custom", Kind: "custom", Addr: c[0]})
	}
	return t
}

func startDNSTest() bool {
	dnsTestMu.Lock()
	if dnsTest.Running {
		dnsTestMu.Unlock()
		return false
	}
	dnsTest = DNSTestState{Running: true}
	dnsTestMu.Unlock()
	go safely("dns test", runDNSTest)
	return true
}

func runDNSTest() {
	s := getSettings()
	present := map[string]Adapter{}
	for _, a := range getAdapters(true) {
		present[a.Name] = a
	}
	var lines []Adapter
	names := map[string]string{}
	for _, l := range s.Links {
		if a, ok := present[l.Name]; ok && l.Enabled && !isDead(l.Name) {
			lines = append(lines, a)
			names[a.Name] = labelFor(s, l.Name)
		}
	}
	if len(lines) == 0 {
		dnsTestMu.Lock()
		dnsTest = DNSTestState{Error: "No working connection to test through. Plug one in and try again."}
		dnsTestMu.Unlock()
		return
	}
	targets := buildTargets()
	total := len(targets) * len(lines)
	dnsTestMu.Lock()
	dnsTest.Total, dnsTest.LineName = total, names
	for _, a := range lines {
		dnsTest.Lines = append(dnsTest.Lines, a.Name)
	}
	dnsTestMu.Unlock()

	results := make([]map[string]DNSLineStat, len(targets))
	for i := range results {
		results[i] = map[string]DNSLineStat{}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, a := range lines {
		wg.Add(1)
		go func(a Adapter) {
			defer wg.Done()
			sem := make(chan struct{}, 3)
			var lw sync.WaitGroup
			for i, t := range targets {
				lw.Add(1)
				sem <- struct{}{}
				go func(i int, t dnsTarget) {
					defer lw.Done()
					defer func() { <-sem }()
					var st DNSLineStat
					safely("dns measure", func() { st = measureDNS(a, t) })
					mu.Lock()
					results[i][a.Name] = st
					mu.Unlock()
					dnsTestMu.Lock()
					dnsTest.Progress++
					dnsTestMu.Unlock()
				}(i, t)
			}
			lw.Wait()
		}(a)
	}
	wg.Wait()

	rows := aggregateDNS(targets, results)
	rec := recommendDNS(rows)
	dnsTestMu.Lock()
	dnsTest.Rows, dnsTest.Rec, dnsTest.Running, dnsTest.Done = rows, rec, false, time.Now().Unix()
	dnsTestMu.Unlock()
}

func aggregateDNS(targets []dnsTarget, results []map[string]DNSLineStat) []DNSRow {
	// a line on which nothing answered at all is a line that is down, not a verdict on the servers
	dead := map[string]bool{}
	lineSet := map[string]bool{}
	for _, m := range results {
		for l := range m {
			lineSet[l] = true
		}
	}
	for l := range lineSet {
		any := false
		for _, m := range results {
			if m[l].Median >= 0 {
				any = true
			}
		}
		dead[l] = !any
	}
	var rows []DNSRow
	for i, t := range targets {
		r := DNSRow{ID: t.ID, Label: t.Label, Provider: t.Provider, Kind: t.Kind, Lines: results[i], Median: -1}
		var sum, fail float64
		n := 0
		for l, st := range results[i] {
			if dead[l] {
				continue
			}
			if t.Kind == "router" && st.N == 0 {
				continue // no gateway known on that line
			}
			n++
			fail = maxf(fail, st.Fail)
			if st.Median >= 0 {
				sum += st.Median
			} else {
				sum += 1000
			}
			r.Hijack = r.Hijack || st.Hijack
		}
		if n > 0 {
			r.Median, r.Fail = sum/float64(n), fail
			if r.Fail >= 100 {
				r.Median = -1
			}
		}
		rows = append(rows, r)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Median < 0) != (b.Median < 0) {
			return b.Median < 0
		}
		return a.Median < b.Median
	})
	return rows
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// recommendDNS picks the fastest reliable server and only suggests a change when it is clearly better.
func recommendDNS(rows []DNSRow) *DNSRec {
	var cur *DNSRow
	for i := range rows {
		if rows[i].Kind == "router" {
			cur = &rows[i]
		}
	}
	curMs := -1.0
	if cur != nil {
		curMs = cur.Median
	}
	var best *DNSRow
	plain := map[string]float64{}
	for i := range rows {
		r := &rows[i]
		if r.Kind == "plain" {
			plain[r.Provider] = r.Median
		}
		if r.Kind == "router" || r.Median < 0 || r.Fail > 10 || r.Hijack {
			continue
		}
		if best == nil || r.Median < best.Median {
			best = r
		}
	}
	rec := &DNSRec{Current: curMs, Median: -1}
	if cur != nil && cur.Hijack {
		rec.Notes = append(rec.Notes, "Your current DNS answers names that do not exist with an address. That is how some providers redirect you to a search or ad page.")
	}
	if cur != nil && cur.Fail > 10 && cur.Median >= 0 {
		rec.Notes = append(rec.Notes, fmt.Sprintf("Your current DNS failed %.0f%% of the lookups.", cur.Fail))
	}
	if best == nil {
		rec.Text = "No server answered reliably, so I can't recommend one. Check that the connection really has internet, then run the test again."
		return rec
	}
	rec.Provider, rec.Median = best.Provider, best.Median
	rec.Secure = best.Kind == "doh"
	name := best.Label
	if best.Provider == "custom" {
		name = "your own server"
	}
	// prefer encrypted lookups when they cost almost nothing
	if !rec.Secure && best.Provider != "custom" {
		for i := range rows {
			if rows[i].Provider == best.Provider && rows[i].Kind == "doh" && rows[i].Median >= 0 && rows[i].Fail <= 10 && rows[i].Median <= best.Median+25 {
				rec.Secure, rec.Median = true, rows[i].Median
				name = rows[i].Label
			}
		}
	}
	better := curMs < 0 || (cur != nil && (cur.Hijack || cur.Fail > 10)) || rec.Median <= curMs-maxf(8, curMs*0.2)
	if !better {
		rec.Provider = ""
		rec.Text = fmt.Sprintf("Your current DNS is already fast (%.0f ms). The best alternative, %s, would be about the same, so there is no reason to change.", curMs, name)
		return rec
	}
	if curMs < 0 {
		rec.Text = fmt.Sprintf("Recommended: %s, about %.0f ms per lookup. Your current DNS did not answer on the tested lines.", name, rec.Median)
	} else {
		rec.Text = fmt.Sprintf("Recommended: %s, about %.0f ms per lookup, compared with %.0f ms for your current DNS.", name, rec.Median, curMs)
	}
	rec.Notes = append(rec.Notes, "Lookup speed is what was measured. A server that is quicker can still send you to a farther copy of some websites, so downloads from a few sites may differ.")
	return rec
}

// dnsServersAnswer: do the servers the user chose answer through this line? (plain or encrypted, as chosen)
func dnsServersAnswer(a Adapter, s Settings) bool {
	ns, boots := dnsServersFor(s)
	boot := ""
	if len(boots) > 0 {
		boot = boots[0]
	}
	ok := false
	for _, n := range ns {
		if strings.HasPrefix(n, "https://") {
			c := dohClient(a, boot)
			_, rc, an, err := queryDoH(c, n, "www.gstatic.com")
			c.CloseIdleConnections()
			ok = ok || (err == nil && rc == 0 && an > 0)
		} else {
			_, rc, an, err := queryUDP(a, n, "www.gstatic.com")
			ok = ok || (err == nil && rc == 0 && an > 0)
		}
		if ok {
			return true
		}
	}
	return false
}

func bootOf(p dnsProv) string {
	if len(p.Boot) > 0 {
		return p.Boot[0]
	}
	return p.Plain[0]
}
