package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type TestResult struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Mbps   float64 `json:"mbps"`
	Single float64 `json:"single"`
	IP     string  `json:"ip"`
	Error  string  `json:"error,omitempty"`
}

type ScalePoint struct {
	Streams int     `json:"streams"`
	Mbps    float64 `json:"mbps"`
}

type TestState struct {
	Running  bool           `json:"running"`
	Phase    string         `json:"phase"`
	Progress float64        `json:"progress"`
	Live     float64        `json:"live"` // Mbps right now
	Results  []TestResult   `json:"results"`
	Combined float64        `json:"combined"`
	Merged   float64        `json:"merged"`
	Scaling  []ScalePoint   `json:"scaling"`
	Suggest  map[string]int `json:"suggest"`
	Finished time.Time      `json:"finished"`
	Warning  string         `json:"warning"`
}

var (
	testMu sync.Mutex
	test   TestState
)

// Download test files. Servers rate-limit abusive clients, so we check each one and fall back automatically.
var testURLs = []string{
	"https://dl.google.com/linux/direct/google-chrome-stable_current_amd64.deb", // Google edge cache, usually very close
	"https://cloudflaremirrors.com/archlinux/iso/latest/archlinux-x86_64.iso",
	"https://speed.cloudflare.com/__down?bytes=25000000",
	"https://proof.ovh.net/files/100Mb.dat",
}

// probe: can this client really download from url? returns "" if ok, else the reason.
func probe(c *http.Client, url string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.Do(req)
	if err != nil {
		return "no connection"
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	n, _ := io.CopyN(io.Discard, resp.Body, 256<<10)
	if n < 128<<10 {
		return "no data"
	}
	return ""
}

// workingURLs returns up to 3 test files that every link can really download, so the load is spread
// over several servers (none of them sees a burst from us). Falls back to the best partial match.
func workingURLs(clients []*http.Client, names []string) []string {
	var good []string
	best, bestOK := "", -1
	for _, u := range testURLs {
		ok := 0
		var why []string
		for i, c := range clients {
			if r := probe(c, u); r == "" {
				ok++
			} else {
				why = append(why, fmt.Sprintf("%s: %s", names[i], r))
			}
		}
		logf("test file %s -> %d/%d links ok %v", u, ok, len(clients), why)
		if ok == len(clients) {
			good = append(good, u)
			if len(good) == 3 {
				break
			}
		} else if ok > bestOK {
			best, bestOK = u, ok
		}
	}
	if len(good) == 0 {
		if best == "" {
			best = testURLs[0]
		}
		good = []string{best}
	}
	return good
}

func getTest() TestState {
	testMu.Lock()
	defer testMu.Unlock()
	return test
}

func setTest(fn func(t *TestState)) {
	testMu.Lock()
	fn(&test)
	testMu.Unlock()
}

// clientFor returns an HTTP client whose sockets leave through one adapter (ifIndex>0), or the normal route (0).
func clientFor(a *Adapter) *http.Client {
	d := &net.Dialer{Timeout: 6 * time.Second, KeepAlive: 30 * time.Second}
	if a != nil {
		d.Control = bindControl(a.Index)
		if ip := net.ParseIP(a.IP); ip != nil {
			d.LocalAddr = &net.TCPAddr{IP: ip}
		}
	}
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         d.DialContext,
		TLSClientConfig:     &tls.Config{},
		DisableCompression:  true,
		MaxIdleConnsPerHost: 0,
		DisableKeepAlives:   true,
		ForceAttemptHTTP2:   false,
		TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{}, // force HTTP/1.1 = 1 TCP per stream
	}
	return &http.Client{Transport: tr, Timeout: 0}
}

func publicIP(a *Adapter) string {
	c := clientFor(a)
	c.Timeout = 6 * time.Second
	for _, u := range []string{"https://api.ipify.org", "https://ifconfig.me/ip"} {
		resp, err := c.Get(u)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if ip := strings.TrimSpace(string(b)); net.ParseIP(ip) != nil {
			return ip
		}
	}
	return ""
}

// measure downloads with `streams` parallel connections for `dur` and returns Mbps.
func measure(urls []string, clients []*http.Client, streams int, dur time.Duration, onTick func(mbps float64)) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dur+3*time.Second)
	defer cancel()
	var total int64
	var wg sync.WaitGroup
	var firstErr atomic.Value
	start := time.Now()
	stop := make(chan struct{})
	for i := 0; i < streams; i++ {
		c := clients[i%len(clients)]
		url := urls[(i/len(clients))%len(urls)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
				resp, err := c.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						firstErr.CompareAndSwap(nil, err)
						time.Sleep(300 * time.Millisecond)
					}
					if ctx.Err() != nil {
						return
					}
					continue
				}
				if resp.StatusCode != 200 {
					resp.Body.Close()
					if ctx.Err() == nil {
						firstErr.CompareAndSwap(nil, fmt.Errorf("HTTP %d", resp.StatusCode))
						time.Sleep(800 * time.Millisecond)
					}
					continue
				}
				got := 0
				buf := make([]byte, 64<<10)
				for {
					n, err := resp.Body.Read(buf)
					got += n
					atomic.AddInt64(&total, int64(n))
					if err != nil {
						if got < 64<<10 && ctx.Err() == nil {
							time.Sleep(500 * time.Millisecond) // never hammer a server that cuts us off
						}
						break
					}
					select {
					case <-stop:
						resp.Body.Close()
						return
					default:
					}
				}
				resp.Body.Close()
			}
		}()
	}
	// skip the first 1.5s (TCP slow start) for a fair number
	warm := 1500 * time.Millisecond
	var base int64
	var baseAt time.Time
	tick := time.NewTicker(250 * time.Millisecond)
	last, lastAt := int64(0), time.Now()
	for time.Since(start) < dur {
		<-tick.C
		cur := atomic.LoadInt64(&total)
		if baseAt.IsZero() && time.Since(start) >= warm {
			base, baseAt = cur, time.Now()
		}
		if onTick != nil {
			onTick(float64(cur-last) * 8 / time.Since(lastAt).Seconds() / 1e6)
		}
		last, lastAt = cur, time.Now()
	}
	tick.Stop()
	end := atomic.LoadInt64(&total)
	close(stop)
	cancel()
	wg.Wait()
	if baseAt.IsZero() {
		return 0, fmt.Errorf("test too short")
	}
	mbps := float64(end-base) * 8 / time.Since(baseAt).Seconds() / 1e6
	if end == 0 {
		if e, ok := firstErr.Load().(error); ok {
			return 0, e
		}
		return 0, fmt.Errorf("no data received")
	}
	return mbps, nil
}

func runSpeedTest() {
	testMu.Lock()
	if test.Running {
		testMu.Unlock()
		return
	}
	test = TestState{Running: true, Phase: "Preparing", Suggest: map[string]int{}}
	testMu.Unlock()

	s := getSettings()
	adapters := getAdapters(true)
	var targets []Adapter
	labels := map[string]string{}
	for _, l := range s.Links {
		for _, a := range adapters {
			if a.Name == l.Name && l.Enabled {
				targets = append(targets, a)
				labels[a.Name] = l.Label
			}
		}
	}
	// choose a test file that every link can really download (servers sometimes rate-limit one of our IPs)
	setTest(func(t *TestState) { t.Phase = "Choosing a test server" })
	var probeClients []*http.Client
	var probeNames []string
	for i := range targets {
		probeClients = append(probeClients, clientFor(&targets[i]))
		probeNames = append(probeNames, labels[targets[i].Name])
	}
	url := workingURLs(probeClients, probeNames)
	scale := []int{1, 2, 4, 8, 16, 32}
	steps := float64(len(targets)*3 + 1 + len(scale))
	step := 0.0
	adv := func(phase string) {
		step++
		setTest(func(t *TestState) { t.Phase, t.Progress = phase, step/steps })
	}
	live := func(m float64) { setTest(func(t *TestState) { t.Live = m }) }

	var results []TestResult
	ips := map[string]string{}
	for i := range targets {
		a := targets[i]
		adv("Checking " + labels[a.Name])
		ip := publicIP(&a)
		ips[a.Name] = ip
		statsMu.Lock()
		publicIPs[a.Name] = ip
		statsMu.Unlock()

		adv("Testing " + labels[a.Name] + " (1 stream)")
		single, _ := measure(url, []*http.Client{clientFor(&a)}, 1, 6*time.Second, live)
		adv("Testing " + labels[a.Name] + " (8 streams)")
		mbps, err := measure(url, []*http.Client{clientFor(&a)}, 8, 8*time.Second, live)
		r := TestResult{Name: a.Name, Label: labels[a.Name], Mbps: round1(mbps), Single: round1(single), IP: ip}
		if mbps < 0.5 {
			r.Error = "no data"
			if err != nil {
				r.Error = err.Error()
			}
		}
		results = append(results, r)
		setTest(func(t *TestState) { t.Results = append([]TestResult(nil), results...) })
	}

	// all links at the same time, straight out of each adapter = the real ceiling
	adv("Testing all connections together")
	var cl []*http.Client
	for i := range targets {
		cl = append(cl, clientFor(&targets[i]))
	}
	combined := 0.0
	if len(cl) > 0 {
		combined, _ = measure(url, cl, 8*len(cl), 8*time.Second, live)
	}

	// through the merge (normal route) with a growing number of parallel streams
	merged := 0.0
	var scaling []ScalePoint
	running, _, _, _ := engine.Status()
	for _, n := range scale {
		adv(fmt.Sprintf("Testing through NetMerge (%d streams)", n))
		if !running {
			continue
		}
		m, _ := measure(url, []*http.Client{clientFor(nil)}, n, 7*time.Second, live)
		scaling = append(scaling, ScalePoint{Streams: n, Mbps: round1(m)})
		if m > merged {
			merged = m
		}
	}

	// suggested weights: proportional to measured speed, slowest = 1, max 5
	capOf := func(r TestResult) float64 {
		if r.Single > r.Mbps {
			return r.Single
		}
		return r.Mbps
	}
	min := 0.0
	for _, r := range results {
		if c := capOf(r); c > 0 && (min == 0 || c < min) {
			min = c
		}
	}
	sug := map[string]int{}
	for _, r := range results {
		w := 1
		if c := capOf(r); min > 0 && c > 0 {
			w = int(c/min + 0.5)
		}
		if w < 1 {
			w = 1
		}
		if w > 5 {
			w = 5
		}
		sug[r.Name] = w
	}
	warn := ""
	seen := map[string]string{}
	for n, ip := range ips {
		if ip == "" {
			continue
		}
		if other, ok := seen[ip]; ok {
			warn = fmt.Sprintf("%s and %s use the same internet line (same public IP %s) - merging them will not add speed.", labels[other], labels[n], ip)
		}
		seen[ip] = n
	}
	setTest(func(t *TestState) {
		t.Running, t.Phase, t.Progress, t.Live = false, "Done", 1, 0
		t.Combined, t.Merged, t.Suggest, t.Finished, t.Warning = round1(combined), round1(merged), sug, time.Now(), warn
		t.Scaling = scaling
	})
	logf("speed test (%v): links=%+v combined=%.1f merged-best=%.1f scaling=%+v suggest=%v", url, results, combined, merged, scaling, sug)
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
