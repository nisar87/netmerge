// enginebench compares network engines the way NetMerge uses them: each engine runs as its own process with the
// same settings, captures traffic through its virtual adapter, and the same measurements are taken through it.
// Engines take turns (A B A B ...) so a change in the internet's speed affects both alike. Windows, administrator.
//
//	enginebench -engine "mihomo=C:\mihomo.exe" -engine "netmerge=C:\nmengine.exe" -rounds 3
package main

import (
	"bytes"
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
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type engineFlag []string

func (e *engineFlag) String() string     { return strings.Join(*e, ",") }
func (e *engineFlag) Set(v string) error { *e = append(*e, v); return nil }

const (
	apiAddr   = "127.0.0.1:9797"
	secret    = "bench"
	bigBytes  = 100_000_000
	partBytes = 25_000_000
)

type result map[string][]float64 // metric -> values over rounds

var metrics = []struct {
	key, label, unit string
	higherBetter     bool
}{
	{"start", "Start until the first captured connection works", "ms", false},
	{"conn", "New connection, small web page (median of 30)", "ms", false},
	{"conn_p90", "New connection, small web page (90th percentile)", "ms", false},
	{"dns", "DNS answer (median of 30)", "ms", false},
	{"tls", "HTTPS first byte, new connection each time (median of 20)", "ms", false},
	{"udp", "UDP round trip, NTP (median of 20)", "ms", false},
	{"single", "One download, 100 MB", "Mbit/s", true},
	{"parallel", "8 downloads at once, 8 x 25 MB", "Mbit/s", true},
	{"mem", "Engine memory at the end (working set)", "MB", false},
	{"mem_peak", "Engine memory, peak", "MB", false},
	{"cpu", "Engine CPU time for the whole round", "s", false},
}

func main() {
	var engines engineFlag
	flag.Var(&engines, "engine", "name=path to an engine program (repeat)")
	rounds := flag.Int("rounds", 3, "rounds per engine")
	flag.Parse()
	ifName, err := defaultInterface()
	if err != nil {
		fail("%v", err)
	}
	fmt.Printf("default line: %s\n", ifName)
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
		if r%2 == 1 { // alternate who goes first
			for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
				order[i], order[j] = order[j], order[i]
			}
		}
		for _, e := range order {
			name, path, _ := strings.Cut(e, "=")
			fmt.Printf("\n== round %d: %s ==\n", r+1, name)
			res, err := runRound(name, path, ifName)
			if err != nil {
				fmt.Printf("round failed: %v\n", err)
				continue
			}
			for k, v := range res {
				results[name][k] = append(results[name][k], v)
				fmt.Printf("  %-9s %.1f\n", k, v)
			}
		}
	}
	report(names, results, base)
}

func fail(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }

// baseline: the same small requests without any engine, to show what the internet itself takes
func baseline() map[string]float64 {
	c := newClient(nil)
	var conn []float64
	for i := 0; i < 15; i++ {
		if d, err := timeGet(c, "http://1.1.1.1/cdn-cgi/trace"); err == nil {
			conn = append(conn, d)
		}
	}
	return map[string]float64{"conn": median(conn)}
}

func config(name, ifName string) []byte {
	routes := []string{"1.1.1.1/32", "162.159.200.0/24"}
	tun := map[string]any{"enable": true, "auto-route": true, "auto-detect-interface": true, "dns-hijack": []string{"any:53"},
		"route-address": routes}
	if name != "netmerge" {
		tun["stack"] = "mixed" // what NetMerge uses with mihomo by default
		tun["route-address"] = append(routes, "198.18.0.0/16")
	}
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "ipv6": false, "allow-lan": false, "find-process-mode": "always",
		"unified-delay": true, "tcp-concurrent": false, "external-controller": apiAddr, "secret": secret,
		"profile": map[string]any{"store-selected": false, "store-fake-ip": false},
		"tun":     tun,
		"proxies": []map[string]any{{"name": "LINK1", "type": "direct", "interface-name": ifName, "udp": true}},
		"proxy-groups": []map[string]any{
			{"name": "SLOT1", "type": "select", "proxies": []string{"LINK1"}},
			{"name": "AUTO", "type": "load-balance", "strategy": "round-robin", "proxies": []string{"SLOT1"}},
		},
		"rules": []string{"DOMAIN-SUFFIX,blocked.example,REJECT", "MATCH,AUTO"},
		"sniffer": map[string]any{"enable": true, "force-dns-mapping": true, "parse-pure-ip": true, "override-destination": false,
			"sniff": map[string]any{"HTTP": map[string]any{"ports": []any{80}}, "TLS": map[string]any{"ports": []any{443}}}},
		"dns": map[string]any{"enable": true, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"8.8.8.8", "1.0.0.1"}},
	}
	b, _ := json.Marshal(cfg)
	return b
}

func runRound(name, path, ifName string) (map[string]float64, error) {
	dir, _ := os.MkdirTemp("", "bench-"+name)
	defer os.RemoveAll(dir)
	os.WriteFile(filepath.Join(dir, "config.yaml"), config(name, ifName), 0o644)
	logf, _ := os.Create(filepath.Join(os.TempDir(), "bench-"+name+".log"))
	defer logf.Close()
	start := time.Now()
	cmd := exec.Command(path, "-d", dir)
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		stop(cmd)
		time.Sleep(2 * time.Second) // let the adapter go away before the next engine
	}()
	res := map[string]float64{}
	c := newClient(nil)
	ok := false
	for time.Since(start) < 30*time.Second {
		if _, err := timeGetT(c, "http://1.1.1.1/cdn-cgi/trace", 2*time.Second); err == nil {
			ok = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ok {
		return nil, fmt.Errorf("no captured connection within 30 s")
	}
	res["start"] = ms(time.Since(start))
	time.Sleep(time.Second)

	var conn []float64
	for i := 0; i < 30; i++ {
		if d, err := timeGet(c, "http://1.1.1.1/cdn-cgi/trace"); err == nil {
			conn = append(conn, d)
		}
	}
	res["conn"], res["conn_p90"] = median(conn), pct(conn, 0.9)

	var dns []float64
	var fake netip.Addr
	for i := 0; i < 30; i++ {
		d, a, err := dnsQuery(fmt.Sprintf("bench%d-%d.example.com", i, time.Now().UnixNano()%1e6))
		if err == nil {
			dns = append(dns, d)
		}
		_ = a
	}
	res["dns"] = median(dns)
	if _, a, err := dnsQuery("speed.cloudflare.com"); err == nil {
		fake = a
	} else {
		return res, fmt.Errorf("dns: %v", err)
	}

	sc := newClient(&fake)
	var tl []float64
	for i := 0; i < 20; i++ {
		if d, err := timeGet(sc, "https://speed.cloudflare.com/__down?bytes=1000"); err == nil {
			tl = append(tl, d)
		}
	}
	res["tls"] = median(tl)

	var udp []float64
	for i := 0; i < 20; i++ {
		if d, err := ntp(); err == nil {
			udp = append(udp, d)
		}
	}
	res["udp"] = median(udp)

	if mbps, err := download(&fake, 1, bigBytes); err == nil {
		res["single"] = mbps
	} else {
		fmt.Println("  single download:", err)
	}
	if mbps, err := download(&fake, 8, partBytes); err == nil {
		res["parallel"] = mbps
	} else {
		fmt.Println("  parallel download:", err)
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
	cmd.Process.Kill()
	<-done
}

func newClient(fake *netip.Addr) *http.Client {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false,
		TLSClientConfig: &tls.Config{ServerName: "speed.cloudflare.com"}}
	if fake != nil {
		a := *fake
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			_, port, _ := net.SplitHostPort(addr)
			return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(a.String(), port))
		}
	}
	return &http.Client{Timeout: 120 * time.Second, Transport: tr}
}

func timeGet(c *http.Client, u string) (float64, error) { return timeGetT(c, u, 10*time.Second) }

// timeGetT: a new connection, the request, and the first byte of the answer
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
	return d, nil
}

func download(fake *netip.Addr, n, size int) (float64, error) {
	c := newClient(fake)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int64
	var firstErr error
	t := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := c.Get(fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", size))
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
	if firstErr != nil || total < int64(n*size) {
		return 0, fmt.Errorf("got %d of %d bytes: %v", total, n*size, firstErr)
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
	c, err := net.Dial("udp", "1.1.1.1:53")
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
	d := ms(time.Since(t))
	r = r[:n]
	// the first A record at the end of the answer
	if n >= 4 && bytes.Contains(r, []byte{0, 1, 0, 1}) && binary.BigEndian.Uint16(r[6:8]) > 0 {
		a, _ := netip.AddrFromSlice(r[n-4:])
		return d, a, nil
	}
	return d, netip.Addr{}, nil
}

func ntp() (float64, error) {
	c, err := net.Dial("udp", "162.159.200.1:123")
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
	fmt.Fprintf(&b, "## Engine comparison (median over rounds)\n\nWithout any engine, a new connection to the small page takes %.1f ms.\n\n", base["conn"])
	fmt.Fprintf(&b, "| Measurement | %s | Better |\n|---|%s---|\n", strings.Join(names, " | "), strings.Repeat("---|", len(names)))
	for _, m := range metrics {
		var vals []float64
		row := "| " + m.label + " (" + m.unit + ") |"
		for _, n := range names {
			v := median(res[n][m.key])
			vals = append(vals, v)
			row += fmt.Sprintf(" %.1f |", v)
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
			if diff < 5 {
				best = "about the same"
			} else {
				best = fmt.Sprintf("%s by %.0f%%", names[i], diff)
			}
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
