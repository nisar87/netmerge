// enginebench compares network engines the way NetMerge uses them: each engine runs as its own process with the
// same settings, captures traffic through its virtual adapter, and the same measurements are taken through it.
// Engines take turns (A B A B ...) so changes in the network affect both alike.
//
//	enginebench -engine "mihomo=C:\mihomo.exe" -engine "netmerge=C:\nmengine.exe" -rounds 3      (internet, Windows)
//	enginebench -local -engine "mihomo=/x/mihomo" -engine "netmerge=/x/nmengine" -rounds 3       (a private test
//	   network with its own servers, Linux as root: no internet noise, so small differences show)
package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type engineFlag []string

func (e *engineFlag) String() string     { return strings.Join(*e, ",") }
func (e *engineFlag) Set(v string) error { *e = append(*e, v); return nil }

const (
	goZip   = "https://dl.google.com/go/go1.24.0.windows-amd64.zip"
	apiAddr = "127.0.0.1:9797"
	secret  = "bench"
)

// targets: what is measured, on the internet or on the private test network
type targets struct {
	ifName     string
	routes     []string
	small      string // small page by address (no DNS)
	dnsServer  string // a captured address answering DNS (port 53)
	dnsNames   []string
	tlsURL     string // small HTTPS page by name, new connection each time
	udpAddr    string
	udpNTP     bool // udpAddr is an NTP server (else: an echo server)
	downloads  []dl
	insecure   bool
	noBaseline bool // the test servers are reachable only through an engine
	dnsControl func(string, string, syscall.RawConn) error
}

type dl struct {
	key, label, url string
	n               int
	size            int64
}

type result map[string][]float64

type metric struct {
	key, label, unit string
	higherBetter     bool
}

var baseMetrics = []metric{
	{"start", "Start until the first captured connection works", "ms", false},
	{"conn", "New connection, small page (median of 40)", "ms", false},
	{"conn_p90", "New connection, small page (90th percentile)", "ms", false},
	{"tls", "HTTPS by name, first byte, new connection each time (median of 30)", "ms", false},
	{"udp", "UDP round trip, new socket each time (median of 30)", "ms", false},
}

var tailMetrics = []metric{
	{"mem", "Engine memory at the end (working set)", "MB", false},
	{"mem_peak", "Engine memory, peak", "MB", false},
	{"cpu", "Engine CPU time for the whole round", "s", false},
}

var tg targets

func main() {
	var engines engineFlag
	flag.Var(&engines, "engine", "name=path to an engine program (repeat)")
	rounds := flag.Int("rounds", 3, "rounds per engine")
	local := flag.Bool("local", false, "use a private test network with its own servers (Linux, root)")
	serve := flag.Bool("serve", false, "internal: run the test servers")
	flag.Parse()
	if *serve {
		runServers()
		return
	}
	if *local {
		cleanup, err := setupLocal()
		if err != nil {
			fail("test network: %v", err)
		}
		defer cleanup()
	} else {
		ifName, err := defaultInterface()
		if err != nil {
			fail("%v", err)
		}
		// Google's download servers and Cloudflare: fast, near GitHub's machines, and they keep files whole.
		// Everything of theirs is captured, so names (looked up by Windows itself) lead through the engine.
		tg = targets{ifName: ifName, routes: []string{"1.1.1.1/32", "162.159.200.0/24", "104.16.0.0/12", "142.250.0.0/15", "172.217.0.0/16", "216.58.192.0/19", "74.125.0.0/16", "34.0.0.0/8", "35.0.0.0/8"},
			small: "http://1.1.1.1/cdn-cgi/trace", tlsURL: "https://www.google.com/generate_204", udpAddr: "162.159.200.1:123", udpNTP: true,
			downloads: []dl{
				{"single", "One download, Go for Windows zip (Google)", goZip, 1, 0},
				{"parallel", "8 downloads at once, Go zip x 8 (Google)", goZip, 8, 0},
				{"single_cf", "One download, 25 MB (Cloudflare)", "https://speed.cloudflare.com/__down?bytes=25000000", 1, 25e6},
			}}
	}
	fmt.Printf("line: %s\n", tg.ifName)
	results := map[string]result{}
	var names []string
	for _, e := range engines {
		n, _, _ := strings.Cut(e, "=")
		names = append(names, n)
		results[n] = result{}
	}
	base := baseline()
	for r := 0; r < *rounds; r++ {
		order := append([]string(nil), engines...)
		if r%2 == 1 {
			for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
				order[i], order[j] = order[j], order[i]
			}
		}
		for _, e := range order {
			name, path, _ := strings.Cut(e, "=")
			fmt.Printf("\n== round %d: %s ==\n", r+1, name)
			res, err := runRound(name, path)
			if err != nil {
				fmt.Printf("round failed: %v\n", err)
			}
			keys := make([]string, 0, len(res))
			for k := range res {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				results[name][k] = append(results[name][k], res[k])
				fmt.Printf("  %-9s %.1f\n", k, res[k])
			}
		}
	}
	report(names, results, base)
}

func fail(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }

func baseline() map[string]float64 {
	if tg.small == "" || tg.noBaseline {
		return nil
	}
	c := newClient()
	var conn []float64
	for i := 0; i < 15; i++ {
		if d, err := timeGet(c, tg.small); err == nil {
			conn = append(conn, d)
		}
	}
	return map[string]float64{"conn": median(conn)}
}

// config mirrors what NetMerge writes (see config.go in the app): DNS is left to the PC (the engine's own DNS is
// off), names come from the TLS/HTTP sniffer, only the test addresses are captured.
func config(name string) []byte {
	tun := map[string]any{"enable": true, "auto-route": true, "auto-detect-interface": true, "route-address": tg.routes, "mtu": 9000}
	if name != "netmerge" {
		tun["stack"] = "mixed" // what NetMerge uses with mihomo by default
		if g := os.Getenv("BENCH_MIHOMO_GSO"); g != "" {
			tun["gso"] = g == "1"
		}
	}
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "ipv6": false, "allow-lan": false, "find-process-mode": envOr("BENCH_FIND", "always"),
		"unified-delay": true, "tcp-concurrent": false, "external-controller": apiAddr, "secret": secret,
		"profile": map[string]any{"store-selected": false, "store-fake-ip": false},
		"tun":     tun,
		"dns":     map[string]any{"enable": false},
		"proxies": []map[string]any{{"name": "LINK1", "type": "direct", "interface-name": tg.ifName, "udp": true}},
		"proxy-groups": []map[string]any{
			{"name": "SLOT1", "type": "select", "proxies": []string{"LINK1"}},
			{"name": "AUTO", "type": "load-balance", "strategy": "round-robin", "proxies": []string{"SLOT1"}},
		},
		"rules": []string{"DOMAIN-SUFFIX,blocked.example,REJECT", "MATCH,AUTO"},
		"sniffer": map[string]any{"enable": true, "force-dns-mapping": true, "parse-pure-ip": true, "override-destination": false,
			"sniff": map[string]any{"HTTP": map[string]any{"ports": []any{80, "8080-8880"}}, "TLS": map[string]any{"ports": []any{443}}}},
	}
	b, _ := json.Marshal(cfg)
	return append([]byte("# Generated by NetMerge - edit settings in the app, not here.\n"), b...)
}

func runRound(name, path string) (map[string]float64, error) {
	dir, _ := os.MkdirTemp("", "bench-"+name)
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "config.yaml"), config(name), 0o644)
	logf, _ := os.Create(filepath.Join(os.TempDir(), "bench-"+name+".log"))
	defer logf.Close()
	start := time.Now()
	cmd := exec.Command(path, "-d", dir)
	cmd.Env = append(os.Environ(), "NM_PPROF=127.0.0.1:6060", "NM_TRACE=1")
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		stop(cmd)
		time.Sleep(2 * time.Second) // let the adapter go away before the next engine
	}()
	res := map[string]float64{}
	c := newClient()
	ok := false
	// started = a connection that really went through the engine (it is in the engine's log); one that went
	// past it (before its routes were in place) does not count
	u, _ := url.Parse(tg.small)
	mark := u.Host
	if u.Port() == "" {
		mark += ":80"
	}
	for time.Since(start) < 30*time.Second {
		if _, err := timeGetT(c, tg.small, 2*time.Second); err == nil {
			if b, _ := os.ReadFile(logf.Name()); strings.Contains(string(b), "--> "+mark) {
				ok = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ok {
		return nil, fmt.Errorf("no captured connection within 30 s")
	}
	res["start"] = ms(time.Since(start))
	time.Sleep(time.Second)

	var conn []float64
	for i := 0; i < 40; i++ {
		if d, err := timeGet(c, tg.small); err == nil {
			conn = append(conn, d)
		}
	}
	res["conn"], res["conn_p90"] = median(conn), pct(conn, 0.9)

	var tl []float64
	for i := 0; i < 30; i++ {
		if d, err := timeGet(c, tg.tlsURL); err == nil {
			tl = append(tl, d)
		}
	}
	res["tls"] = median(tl)

	var udp []float64
	for i := 0; i < 30; i++ {
		if d, err := udpRT(); err == nil {
			udp = append(udp, d)
		}
	}
	res["udp"] = median(udp)

	for _, d := range tg.downloads {
		var prof chan []byte
		if d.key == "parallel" && name == "netmerge" && os.Getenv("BENCH_PPROF") != "" {
			prof = make(chan []byte, 1)
			go func() { prof <- fetch("http://127.0.0.1:6060/debug/pprof/profile?seconds=5") }()
		}
		if mbps, err := download(d.url, d.n, d.size); err == nil {
			res[d.key] = mbps
		} else {
			fmt.Printf("  %s: %v\n", d.key, err)
		}
		if prof != nil {
			if b := <-prof; len(b) > 0 {
				os.WriteFile(filepath.Join(os.TempDir(), fmt.Sprintf("netmerge-cpu-%d.pprof", time.Now().UnixNano())), b, 0o644)
			}
		}
	}
	// how many connections the engine logged (a check that the measured traffic really went through it)
	if b, err := os.ReadFile(logf.Name()); err == nil {
		res["logged_tcp"] = float64(strings.Count(string(b), "[TCP]"))
		dests := map[string]int{}
		for _, l := range strings.Split(string(b), "\n") {
			if i := strings.Index(l, "--> "); i > 0 && strings.Contains(l, "[TCP]") {
				d := strings.Fields(l[i+4:])[0]
				dests[d]++
			} else if strings.Contains(l, "level=warning") || strings.Contains(l, "level=error") || strings.Contains(l, "[TRACE] setup") || strings.Contains(l, "ready in") {
				fmt.Println("  log:", l)
			}
		}
		fmt.Printf("  connections logged by destination: %v\n", dests)
		// NetMerge's engine: how long each step of a new connection took (median)
		steps := map[string][]float64{}
		for _, l := range strings.Split(string(b), "\n") {
			i := strings.Index(l, "[TRACE] ")
			if i < 0 {
				continue
			}
			f := strings.Fields(strings.TrimSuffix(l[i:], "\""))
			for _, kv := range f[3:] {
				if k, v, ok := strings.Cut(kv, "="); ok {
					if us, err := strconv.ParseFloat(strings.TrimSuffix(v, "us"), 64); err == nil {
						steps[f[1]+" "+k] = append(steps[f[1]+" "+k], us/1000)
					}
				}
			}
		}
		keys := make([]string, 0, len(steps))
		for k := range steps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  step %-22s median %.2f ms, 90%% %.2f ms (%d)\n", k, median(steps[k]), pct(steps[k], 0.9), len(steps[k]))
		}
	}
	if ws, peak, cpu, err := procStats(cmd.Process.Pid); err == nil {
		res["mem"], res["mem_peak"], res["cpu"] = float64(ws)/1e6, float64(peak)/1e6, cpu.Seconds()
	}
	return res, nil
}

func stop(cmd *exec.Cmd) {
	req, _ := http.NewRequest("POST", "http://"+apiAddr+"/quit", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	if resp, err := (&http.Client{Timeout: 2 * time.Second}).Do(req); err == nil && resp.StatusCode < 300 {
		resp.Body.Close()
		select {
		case <-done:
			return
		case <-time.After(5 * time.Second):
		}
	}
	interrupt(cmd)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		<-done
	}
}

// newClient opens a new connection per request. Names are looked up outside the engines, as on the PC (its DNS
// server is the router, which NetMerge keeps out of the capture): on the internet with the system's resolver,
// on the test network with its DNS server over the cable.
func newClient() *http.Client {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: tg.insecure},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, _ := net.SplitHostPort(addr)
			if _, err := netip.ParseAddr(host); err != nil && tg.dnsServer != "" {
				_, a, err := dnsQuery(host)
				if err != nil || !a.IsValid() {
					return nil, fmt.Errorf("lookup %s failed: %v", host, err)
				}
				host = a.String()
			}
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(host, port))
		}}
	return &http.Client{Timeout: 180 * time.Second, Transport: tr}
}

func fetch(u string) []byte {
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Get(u)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b
}

func timeGet(c *http.Client, u string) (float64, error) { return timeGetT(c, u, 10*time.Second) }

// timeGetT: the name lookup, a new connection, the request, and the first byte of the answer
func timeGetT(c *http.Client, u string, to time.Duration) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	t := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	d := ms(time.Since(t))
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return d, nil
}

func download(u string, n int, size int64) (float64, error) {
	c := newClient()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	var firstErr error
	t := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(u)
			if err == nil {
				var got int64
				got, err = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				mu.Lock()
				total += got
				mu.Unlock()
			}
			if err != nil {
				mu.Lock()
				firstErr = err
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if size == 0 && total > int64(n)<<20 {
		size = total / int64(n)
	}
	if firstErr != nil || size == 0 || total < int64(n)*size {
		return 0, fmt.Errorf("got %d of %d bytes: %v", total, int64(n)*size, firstErr)
	}
	return float64(total) * 8 / time.Since(t).Seconds() / 1e6, nil
}

func buildQuery(name string) []byte {
	b := []byte{0x12, 0x34, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, l := range strings.Split(name, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0, 0, 1, 0, 1)
}

func dnsQuery(name string) (float64, netip.Addr, error) {
	c, err := (&net.Dialer{Control: tg.dnsControl}).Dial("udp", tg.dnsServer)
	if err != nil {
		return 0, netip.Addr{}, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	t := time.Now()
	c.Write(buildQuery(name))
	r := make([]byte, 1500)
	n, err := c.Read(r)
	if err != nil {
		return 0, netip.Addr{}, err
	}
	return ms(time.Since(t)), firstA(r[:n]), nil
}

// firstA returns the first A record of a DNS answer.
func firstA(r []byte) netip.Addr {
	if len(r) < 12 {
		return netip.Addr{}
	}
	qd, an := int(binary.BigEndian.Uint16(r[4:6])), int(binary.BigEndian.Uint16(r[6:8]))
	p := 12
	skip := func() bool {
		for p < len(r) {
			l := int(r[p])
			if l == 0 {
				p++
				return true
			}
			if l&0xc0 == 0xc0 {
				p += 2
				return true
			}
			p += 1 + l
		}
		return false
	}
	for i := 0; i < qd; i++ {
		if !skip() {
			return netip.Addr{}
		}
		p += 4
	}
	for i := 0; i < an; i++ {
		if !skip() || p+10 > len(r) {
			return netip.Addr{}
		}
		typ, rdlen := binary.BigEndian.Uint16(r[p:]), int(binary.BigEndian.Uint16(r[p+8:]))
		p += 10
		if p+rdlen > len(r) {
			return netip.Addr{}
		}
		if typ == 1 && rdlen == 4 {
			a, _ := netip.AddrFromSlice(r[p : p+4])
			return a
		}
		p += rdlen
	}
	return netip.Addr{}
}

// udpRT: a new socket, one request, its answer (NTP or echo)
func udpRT() (float64, error) {
	c, err := net.Dial("udp", tg.udpAddr)
	if err != nil {
		return 0, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	req := make([]byte, 48)
	req[0] = 0x1b
	t := time.Now()
	c.Write(req)
	if _, err := c.Read(make([]byte, 512)); err != nil {
		return 0, err
	}
	return ms(time.Since(t)), nil
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func median(v []float64) float64 { return pct(v, 0.5) }

func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(p*float64(len(s)-1)+0.5)]
}

func report(names []string, res map[string]result, base map[string]float64) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Engine comparison (median over rounds)\n\n")
	if base != nil {
		fmt.Fprintf(&b, "Without any engine, a new connection to the small page takes %.2f ms.\n\n", base["conn"])
	}
	fmt.Fprintf(&b, "| Measurement | %s | Better |\n|---|%s---|\n", strings.Join(names, " | "), strings.Repeat("---|", len(names)))
	all := append(append([]metric(nil), baseMetrics...), nil...)
	for _, d := range tg.downloads {
		all = append(all, metric{d.key, d.label, "Mbit/s", true})
	}
	all = append(all, tailMetrics...)
	for _, m := range all {
		var vals []float64
		row := "| " + m.label + " (" + m.unit + ") |"
		for _, n := range names {
			v := median(res[n][m.key])
			vals = append(vals, v)
			row += fmt.Sprintf(" %.2f |", v)
		}
		best := ""
		if len(vals) == 2 && vals[0] > 0 && vals[1] > 0 {
			i, j := 0, 1
			if (vals[1] > vals[0]) == m.higherBetter {
				i, j = 1, 0
			}
			diff := (vals[i] - vals[j]) / vals[j] * 100
			if diff < 0 {
				diff = -diff
			}
			if diff < 3 {
				best = "about the same"
			} else {
				best = fmt.Sprintf("%s by %.0f%%", names[i], diff)
			}
		} else if len(vals) == 2 {
			best = "no result for one"
		}
		b.WriteString(row + " " + best + " |\n")
	}
	fmt.Println("\n" + b.String())
	if f := os.Getenv("GITHUB_STEP_SUMMARY"); f != "" {
		if fh, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644); err == nil {
			fh.WriteString(b.String())
			fh.Close()
		}
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
