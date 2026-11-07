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
		c := allocSlots(w, dispatchSlots)
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
	if c := allocSlots([]float64{3, 1}, 8); c[0] != 6 || c[1] != 2 {
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
