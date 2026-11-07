package main

import (
	"strings"
	"testing"
	"time"
)

func TestDeadLinkLeftOut(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	settings.Links = []Link{{Name: "Ethernet", Label: "LAN", Enabled: true, Weight: 3}, {Name: "Wi-Fi", Label: "Wi-Fi", Enabled: true, Weight: 1}}
	// the stub adapter list (non-windows) provides Ethernet + Wi-Fi
	cfg, _ := buildConfig(getSettings())
	if !strings.Contains(string(cfg), `"LINK2"`) {
		t.Skip("no adapters in this environment")
	}
	probeMu.Lock()
	probes["Wi-Fi"] = probeState{dead: true}
	probeMu.Unlock()
	defer func() { probeMu.Lock(); delete(probes, "Wi-Fi"); probeMu.Unlock() }()
	cfg, _ = buildConfig(getSettings())
	s := string(cfg)
	i := strings.Index(s, `"name": "FAST"`)
	if i < 0 || strings.Contains(s[i-300:i+200], `"LINK2"`) && strings.Contains(s[i:i+200], `"LINK2"`) {
		t.Fatalf("dead link still in FAST group: %s", s[i-100:i+200])
	}
	if strings.Contains(s, "health") || strings.Contains(s, `"interval"`) {
		t.Fatal("engine health check still configured")
	}
}

func TestAutoGroup(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	settings.Links = []Link{{Name: "Ethernet", Label: "LAN", Enabled: true, Weight: 3}, {Name: "Wi-Fi", Label: "Wi-Fi", Enabled: true, Weight: 1}}
	cfg, _ := buildConfig(getSettings())
	s := string(cfg)
	if !strings.Contains(s, `"LINK2"`) {
		t.Skip("no adapters in this environment")
	}
	if !strings.Contains(s, `"name": "AUTO"`) || !strings.Contains(s, "MATCH,AUTO") {
		t.Fatalf("smart dispatcher group missing from the config")
	}
	settings.ClassicBalance = true
	cfg, _ = buildConfig(getSettings())
	if strings.Contains(string(cfg), `"name": "AUTO"`) {
		t.Fatal("classic mode must not use AUTO")
	}
}

func TestAllocSlots(t *testing.T) {
	for _, w := range [][]float64{{1, 1}, {9, 1}, {100, 1}, {1, 100}, {5, 3, 2}, {0.1, 0.1, 0.1}, {1}, {0, 5}} {
		c := allocSlots(w, dispatchSlots, nil)
		sum := 0
		for i, n := range c {
			if n < 1 {
				t.Fatalf("%v -> %v: link %d has no slot", w, c, i)
			}
			sum += n
		}
		if sum != dispatchSlots {
			t.Fatalf("%v -> %v: slots add up to %d", w, c, sum)
		}
	}
	if c := allocSlots([]float64{3, 1}, 8, nil); c[0] != 6 || c[1] != 2 {
		t.Fatalf("3:1 should give 6/2, got %v", c)
	}
}

func TestPeriodStart(t *testing.T) {
	loc := time.UTC
	cases := []struct {
		now   time.Time
		reset int
		want  string
	}{
		{time.Date(2026, 11, 7, 12, 0, 0, 0, loc), 1, "2026-11-01"},
		{time.Date(2026, 11, 7, 12, 0, 0, 0, loc), 15, "2026-10-15"},
		{time.Date(2026, 1, 3, 12, 0, 0, 0, loc), 10, "2025-12-10"},
		{time.Date(2026, 11, 15, 0, 0, 0, 0, loc), 15, "2026-11-15"},
	}
	for _, c := range cases {
		if got := periodStart(c.now, c.reset).Format("2006-01-02"); got != c.want {
			t.Fatalf("%v reset %d: got %s want %s", c.now, c.reset, got, c.want)
		}
	}
}

func TestQuotaLeavesLinkOut(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	settings.Links = []Link{{Name: "Ethernet", Label: "LAN", Enabled: true, Weight: 1}, {Name: "Wi-Fi", Label: "Wi-Fi", Enabled: true, Weight: 1, QuotaGB: 1}}
	settings.QuotaAction = "leave"
	usageMu.Lock()
	usageLoaded = true
	usage = map[string]map[string][2]uint64{time.Now().Format("2006-01-02"): {"Wi-Fi": {1.2e9, 0}}}
	usageMu.Unlock()
	quotaCheck()
	if !isQuotaOut("Wi-Fi") || isQuotaOut("Ethernet") {
		t.Fatal("Wi-Fi should be left out after passing its allowance, LAN should not")
	}
	settings.Links[1].QuotaGB = 5
	quotaCheck()
	if isQuotaOut("Wi-Fi") {
		t.Fatal("raising the allowance must bring the connection back")
	}
}

func TestShareConfig(t *testing.T) {
	cfg := map[string]any{"allow-lan": false}
	s := defaultSettings()
	shareConfig(cfg, s)
	if cfg["allow-lan"] != false {
		t.Fatal("sharing must be off by default")
	}
	s.Share = &ShareSettings{Enabled: true, Pass: "abc123"}
	shareConfig(cfg, s)
	if cfg["allow-lan"] != true || cfg["mixed-port"] != 7890 {
		t.Fatalf("sharing config missing: %v", cfg)
	}
	if a, _ := cfg["authentication"].([]string); len(a) != 1 || a[0] != "netmerge:abc123" {
		t.Fatalf("password not set: %v", cfg["authentication"])
	}
	s.Share = &ShareSettings{Enabled: true} // no password yet: stay closed
	cfg = map[string]any{"allow-lan": false}
	shareConfig(cfg, s)
	if cfg["allow-lan"] != false {
		t.Fatal("must not open without a password")
	}
}

func TestShareListener(t *testing.T) {
	cfg := map[string]any{}
	s := defaultSettings()
	s.Share = &ShareSettings{Enabled: true, Pass: "pw12345678"}
	shareConfig(cfg, s)
	ls, _ := cfg["listeners"].([]map[string]any)
	if len(ls) != 1 || ls[0]["type"] != "shadowsocks" || ls[0]["port"] != 8388 || ls[0]["cipher"] != "aes-128-gcm" || ls[0]["password"] != "pw12345678" {
		t.Fatalf("shadowsocks listener wrong: %v", ls)
	}
}

func TestArpParse(t *testing.T) {
	for _, ln := range []string{"  192.168.1.101         4c-eb-b0-ba-2c-e3     dynamic", "  192.168.1.101         4c-eb-b0-ba-2c-e3     dynamisch", "  192.168.1.101         4c-eb-b0-ba-2c-e3     动态"} {
		if m := macRe.FindStringSubmatch(ln); m == nil || m[1] != "192.168.1.101" {
			t.Errorf("no match: %q", ln)
		}
	}
}

func TestBackupRole(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	settings.Links = []Link{{Name: "Ethernet", Label: "LAN", Enabled: true, Weight: 1}, {Name: "Wi-Fi", Label: "Wi-Fi", Enabled: true, Weight: 1, Role: "backup"}}
	ls := activeLinks(getSettings())
	if len(ls) < 2 {
		t.Skip("no adapters in this environment")
	}
	if ls[0].Dead || !ls[1].Dead {
		t.Fatalf("backup should stand by while the main line works: %+v", ls)
	}
	probeMu.Lock()
	probes["Ethernet"] = probeState{dead: true}
	probeMu.Unlock()
	defer func() { probeMu.Lock(); delete(probes, "Ethernet"); probeMu.Unlock() }()
	ls = activeLinks(getSettings())
	if !ls[0].Dead || ls[1].Dead {
		t.Fatalf("backup should take over when the main line is down: %+v", ls)
	}
}

func TestBusyRole(t *testing.T) {
	statsMu.Lock()
	dispLinks["A"] = &dispLink{name: "A", capRx: 100e6 / 8, capTx: 100e6 / 8, emaRx: 10e6 / 8}
	statsMu.Unlock()
	ls := []activeLink{{Link: Link{Name: "A"}}, {Link: Link{Name: "B", Role: "busy"}}}
	if got := applyBusyRole(ls); len(got) != 1 {
		t.Fatalf("idle main line: busy line should stay out, got %d", len(got))
	}
	statsMu.Lock()
	dispLinks["A"].emaRx = 90e6 / 8
	statsMu.Unlock()
	if got := applyBusyRole(ls); len(got) != 2 {
		t.Fatalf("full main line: busy line should join, got %d", len(got))
	}
	statsMu.Lock()
	dispLinks["A"].emaRx = 60e6 / 8
	statsMu.Unlock()
	if got := applyBusyRole(ls); len(got) != 2 {
		t.Fatal("between the limits it must stay on (no flapping)")
	}
	statsMu.Lock()
	dispLinks["A"].emaRx = 20e6 / 8
	statsMu.Unlock()
	if got := applyBusyRole(ls); len(got) != 1 {
		t.Fatal("main line has room again: busy line should leave")
	}
}

func TestPortalResponse(t *testing.T) {
	g := "http://www.gstatic.com/generate_204"
	if portalResponse(g, 204, 0) {
		t.Fatal("204 is the real internet")
	}
	if !portalResponse(g, 302, 0) || !portalResponse(g, 200, 120) {
		t.Fatal("a redirect or a page instead of 204 is a sign-in page")
	}
	if portalResponse(g, 200, 0) {
		t.Fatal("200 with no body is not proof of a portal")
	}
	if portalResponse("http://www.msftconnecttest.com/connecttest.txt", 200, 20) {
		t.Fatal("200 is the normal answer of that address")
	}
}

func TestQuality(t *testing.T) {
	var good, bad []qSample
	for i := 0; i < 20; i++ {
		good = append(good, qSample{true, 20 + i%3})
		bad = append(bad, qSample{i%4 != 0, 150 + (i%2)*120})
	}
	g, b := qualityOf(good), qualityOf(bad)
	if g.Label != "Excellent" {
		t.Fatalf("steady fast line: %+v", g)
	}
	if b.Score >= g.Score || b.Loss < 20 {
		t.Fatalf("lossy jumpy line should score lower: %+v vs %+v", b, g)
	}
	if q := qualityOf(good[:3]); q.Label != "" {
		t.Fatal("too few probes to judge")
	}
}

func TestResumeDetected(t *testing.T) {
	now := time.Now()
	if resumeDetected(now.Add(-2*time.Second), now) || !resumeDetected(now.Add(-90*time.Second), now) || resumeDetected(time.Time{}, now) {
		t.Fatal("sleep detection wrong")
	}
}

func TestCallRules(t *testing.T) {
	s := Settings{AppRules: []AppRule{{Process: "zoom.exe", Target: "fast"}}}
	r := callRules(s, "STABLE")
	for _, x := range r {
		if strings.Contains(strings.ToLower(x), "zoom.exe") {
			t.Fatal("user rule must win")
		}
	}
	if len(r) == 0 {
		t.Fatal("no call rules")
	}
	s.NoCallProtect = true
	if len(callRules(s, "STABLE")) != 0 {
		t.Fatal("should be off")
	}
}

func TestSlowLinkGetsNoSlot(t *testing.T) {
	c := allocSlots([]float64{50, 40, 3}, dispatchSlots, []bool{false, false, true})
	if c[2] != 0 || c[0]+c[1] != dispatchSlots {
		t.Fatalf("got %v", c)
	}
	c = allocSlots([]float64{50, 40, 3}, dispatchSlots, nil)
	if c[2] != 1 {
		t.Fatalf("got %v", c)
	}
}

func TestSlowFlags(t *testing.T) {
	set := []*dispLink{{capRx: 6e6, capTx: 6e6, seenRx: true}, {capRx: 5e6, capTx: 5e6, seenRx: true}, {capRx: 0.4e6, capTx: 0.4e6, seenRx: true}}
	f := slowFlags(set)
	if f[0] || f[1] || !f[2] {
		t.Fatalf("got %v", f)
	}
}

func TestInWindow(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 1, 1, h, m, 0, 0, time.UTC) }
	if !inWindow(at(23, 0), "22:00", "06:00") || !inWindow(at(3, 0), "22:00", "06:00") || inWindow(at(12, 0), "22:00", "06:00") {
		t.Fatal("overnight window")
	}
	if !inWindow(at(10, 0), "09:00", "17:00") || inWindow(at(17, 0), "09:00", "17:00") || inWindow(at(10, 0), "x", "17:00") {
		t.Fatal("day window")
	}
}

func TestPresetApplyRestore(t *testing.T) {
	s := defaultSettings()
	s.Links = []Link{{Name: "Ethernet", Enabled: true, Weight: 1}}
	s.AppRules = []AppRule{{Process: "Zoom.exe", Target: "fast"}}
	orig := append([]AppRule(nil), s.AppRules...)
	p, _ := findPreset(s, "work")
	applyPresetLocked(&s, p)
	zoom := 0
	for _, r := range s.AppRules {
		if strings.EqualFold(r.Process, "zoom.exe") {
			zoom++
			if r.Target != "fast" {
				t.Fatal("user rule must win")
			}
		}
	}
	if zoom != 1 || s.Preset != "work" || s.Block.Enabled {
		t.Fatalf("bad apply: %+v", s.AppRules)
	}
	g, _ := findPreset(s, "gaming") // switching must not pile rules up
	applyPresetLocked(&s, g)
	for _, r := range s.AppRules {
		if strings.EqualFold(r.Process, "mstsc.exe") && s.Preset == "gaming" {
			// default mstsc rule is part of the user's own list in defaultSettings only; here it was replaced
		}
	}
	restoreLocked(&s, s.PresetBackup)
	if len(s.AppRules) != len(orig) || s.Mode != "fast" {
		t.Fatal("restore did not bring back the original")
	}
}

func TestDNSCleaning(t *testing.T) {
	if got := cleanIPs([]string{" 9.9.9.9 ", "bad", "127.0.0.1", "0.0.0.0", "1.1.1.1", "8.8.8.8"}); len(got) != 2 || got[0] != "9.9.9.9" || got[1] != "1.1.1.1" {
		t.Fatalf("ips %v", got)
	}
	if got := cleanDomains([]string{"*.Corp.Example.com", "bad_name", "corp.example.com", "x"}); len(got) != 1 || got[0] != "corp.example.com" {
		t.Fatalf("domains %v", got)
	}
}

func TestDNSConfig(t *testing.T) {
	s := defaultSettings()
	s.BuiltinDNS = true
	d := buildDNSConfig(s) // old built-in behaviour must be unchanged
	if ns := d["nameserver"].([]string); len(ns) != 2 || ns[0] != "1.1.1.1" || ns[1] != "8.8.8.8" {
		t.Fatalf("legacy nameserver %v", ns)
	}
	if _, ok := d["default-nameserver"]; ok {
		t.Fatal("legacy config must not gain keys")
	}
	s.DNSProvider, s.DNSSecure, s.DNSDomains = "quad9", true, []string{"corp.example.com"}
	d = buildDNSConfig(s)
	if ns := d["nameserver"].([]string); ns[0] != "https://9.9.9.9/dns-query" {
		t.Fatalf("doh %v", ns)
	}
	pol := d["nameserver-policy"].(map[string]any)
	if _, ok := pol["+.corp.example.com"]; !ok {
		t.Fatal("policy missing")
	}
	s.DNSProvider, s.DNSSecure, s.DNSCustom = "custom", true, []string{"192.168.1.5"}
	d = buildDNSConfig(s)
	if ns := d["nameserver"].([]string); len(ns) != 1 || ns[0] != "192.168.1.5" {
		t.Fatalf("custom %v", ns)
	}
	s.BuiltinDNS = false
	if dnsActive(s) {
		t.Fatal("must be off when built-in DNS is off")
	}
}

func TestDNSWire(t *testing.T) {
	id, q := dnsQuery("www.example.com")
	resp := append([]byte{}, q[:12]...)
	resp[2], resp[3], resp[7] = 0x81, 0x80, 1 // response, no error, one answer
	if rc, an, ok := parseDNS(id, resp); !ok || rc != 0 || an != 1 {
		t.Fatalf("parse %v %v %v", rc, an, ok)
	}
	if _, _, ok := parseDNS(id+1, resp); ok {
		t.Fatal("must reject another id")
	}
}

func TestDNSRecommend(t *testing.T) {
	mk := func(id, prov, kind string, ms, fail float64, hij bool) DNSRow {
		return DNSRow{ID: id, Provider: prov, Kind: kind, Label: id, Median: ms, Fail: fail, Hijack: hij}
	}
	rows := []DNSRow{mk("router", "windows", "router", 80, 0, false), mk("c:plain", "cloudflare", "plain", 30, 0, false),
		mk("c:doh", "cloudflare", "doh", 45, 0, false), mk("g:plain", "google", "plain", 35, 20, false)}
	r := recommendDNS(rows)
	if r.Provider != "cloudflare" || !r.Secure {
		t.Fatalf("rec %+v", r)
	}
	rows[0].Median = 35 // current is nearly as fast: no change
	if r := recommendDNS(rows); r.Provider != "" {
		t.Fatalf("should not recommend: %+v", r)
	}
	rows[0].Hijack = true // a redirecting provider is a reason to change even if fast
	if r := recommendDNS(rows); r.Provider == "" {
		t.Fatal("should recommend when the current DNS redirects")
	}
	rows[1].Hijack = true
	rows[2].Fail = 50
	if r := recommendDNS(rows); r.Provider != "" && r.Provider != "google" {
		t.Fatalf("unreliable choice %+v", r)
	}
}

func TestDNSAggregate(t *testing.T) {
	targets := []dnsTarget{{ID: "router", Kind: "router"}, {ID: "x", Kind: "plain", Provider: "x"}}
	res := []map[string]DNSLineStat{
		{"A": {Median: 50, N: 12}, "B": {Median: -1, Fail: 100, N: 12}},
		{"A": {Median: 20, N: 12}, "B": {Median: -1, Fail: 100, N: 12}},
	}
	rows := aggregateDNS(targets, res)
	if rows[0].ID != "x" || rows[0].Median != 20 || rows[0].Fail != 0 {
		t.Fatalf("a dead line must not count against the servers: %+v", rows)
	}
}
