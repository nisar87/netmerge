package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Any mix of connections must work the same way: LAN + Wi-Fi, two Wi-Fi, two Wi-Fi + LAN, three LANs, a LAN that
// was swapped for a second Wi-Fi. Nothing may depend on which kind a connection is or on which names it has.

func setAdapters(list ...Adapter) {
	adaptersMu.Lock()
	adapterCache, adapterAt = list, time.Now().Add(time.Hour)
	adaptersMu.Unlock()
}

func ad(name, desc, ip string, idx int) Adapter {
	return Adapter{Name: name, Desc: desc, Index: idx, IP: ip, Gateway: "192.168.1.1", LinkRate: "300 Mbps", Up: true, Kind: kindOf(name, desc)}
}

func freshSettings(t *testing.T) {
	t.Helper()
	rootDir = t.TempDir()
	settings = defaultSettings()
	t.Cleanup(func() {
		adaptersMu.Lock()
		adapterCache, adapterAt = nil, time.Time{}
		adaptersMu.Unlock()
		statsMu.Lock()
		tracks = map[string]*linkTrack{}
		statsMu.Unlock()
		busyMu.Lock()
		busyOn = map[string]bool{}
		busyMu.Unlock()
	})
	probeMu.Lock()
	probes = map[string]probeState{}
	probeMu.Unlock()
}

// validateConfig checks that the engine configuration only points at things that exist.
func validateConfig(t *testing.T, raw []byte, present map[string]bool) map[string]bool {
	t.Helper()
	var cfg struct {
		Proxies []map[string]any `json:"proxies"`
		Groups  []struct {
			Name    string   `json:"name"`
			Proxies []string `json:"proxies"`
		} `json:"proxy-groups"`
		Rules []string `json:"rules"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{"DIRECT": true, "REJECT": true}
	for _, p := range cfg.Proxies {
		n := p["name"].(string)
		if names[n] {
			t.Fatalf("proxy name used twice: %s", n)
		}
		names[n] = true
		if ifn, _ := p["interface-name"].(string); !present[ifn] {
			t.Fatalf("proxy %s is bound to %q, which is not a present connection", n, ifn)
		}
	}
	for _, g := range cfg.Groups {
		if names[g.Name] {
			t.Fatalf("group name used twice: %s", g.Name)
		}
		names[g.Name] = true
	}
	for _, g := range cfg.Groups {
		if len(g.Proxies) == 0 {
			t.Fatalf("group %s is empty", g.Name)
		}
		for _, m := range g.Proxies {
			if !names[m] {
				t.Fatalf("group %s lists %s, which does not exist", g.Name, m)
			}
		}
	}
	for _, r := range cfg.Rules {
		f := strings.Split(r, ",")
		if f[len(f)-1] == "no-resolve" {
			f = f[:len(f)-1]
		}
		if tgt := f[len(f)-1]; !names[tgt] {
			t.Fatalf("rule %q points at %q, which does not exist", r, tgt)
		}
	}
	return names
}

func presentSet(ls ...Adapter) map[string]bool {
	m := map[string]bool{}
	for _, a := range ls {
		m[a.Name] = true
	}
	return m
}

var (
	lan    = ad("Ethernet", "Intel(R) Ethernet Connection (2) I219-V", "192.168.1.10", 11)
	wifi1  = ad("Wi-Fi", "Intel(R) Wi-Fi 6 AX201 160MHz", "192.168.1.11", 7)
	wifi2  = ad("Wi-Fi 2", "TP-Link Wireless USB Adapter", "192.168.1.12", 9)
	lan2   = ad("Ethernet 2", "Realtek USB GbE Family Controller", "192.168.1.13", 12)
	mobile = ad("Ethernet 3", "Remote NDIS based Internet Sharing Device", "192.168.42.2", 14)
)

func TestAnyMixOfConnectionsBuildsAValidConfig(t *testing.T) {
	mixes := map[string][]Adapter{
		"lan+wifi":          {lan, wifi1},
		"two wifi":          {wifi1, wifi2},
		"two wifi + lan":    {lan, wifi1, wifi2},
		"three lan":         {lan, lan2, ad("Ethernet 4", "Realtek PCIe GbE", "192.168.1.14", 15)},
		"two wifi + mobile": {wifi1, wifi2, mobile},
		"four lines":        {lan, wifi1, wifi2, lan2},
	}
	for name, mix := range mixes {
		for _, mode := range []string{"fast", "stable", "link:" + mix[len(mix)-1].Name} {
			for _, classic := range []bool{false, true} {
				freshSettings(t)
				setAdapters(mix...)
				syncLinks(mix)
				settings.Mode, settings.ClassicBalance = mode, classic
				settings.AppRules = append(settings.AppRules, AppRule{Process: "game.exe", Target: "link:" + mix[0].Name}, AppRule{Process: "x.exe", Target: "stable"})
				raw, err := buildConfig(getSettings())
				if err != nil {
					t.Fatal(err)
				}
				t.Run(fmt.Sprintf("%s/%s/classic=%v", name, mode, classic), func(t *testing.T) {
					names := validateConfig(t, raw, presentSet(mix...))
					for i := range mix {
						if !names[linkProxyName(i)] {
							t.Errorf("connection %d has no entry", i)
						}
					}
				})
			}
		}
	}
}

func TestSwappingTheLANForASecondWifi(t *testing.T) {
	freshSettings(t)
	setAdapters(lan, wifi1)
	syncLinks(getAdapters(false))
	updateSettings(func(s *Settings) {
		for i := range s.Links {
			if s.Links[i].Name == "Ethernet" {
				s.Links[i].Role, s.Links[i].Weight, s.Links[i].QuotaGB = "busy", 3, 100
			}
		}
		s.Mode, s.VPNLink = "link:Ethernet", "Ethernet"
		s.AppRules = append(s.AppRules, AppRule{Process: "steam.exe", Target: "link:Ethernet"})
	})
	// the cable is moved: now a second Wi-Fi adapter is the other line
	setAdapters(wifi1, wifi2)
	syncLinks(getAdapters(false))
	s := getSettings()
	labels := map[string]bool{}
	for _, l := range s.Links {
		if labels[l.Label] {
			t.Fatalf("two connections have the same label %q", l.Label)
		}
		labels[l.Label] = true
	}
	var found bool
	for _, l := range s.Links {
		if l.Name == "Wi-Fi 2" && l.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("the second Wi-Fi was not picked up as a new connection")
	}
	raw, err := buildConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	validateConfig(t, raw, presentSet(wifi1, wifi2))
	act := activeLinks(s)
	if len(act) != 2 {
		t.Fatalf("expected the two Wi-Fi lines, got %d", len(act))
	}
	for _, l := range act {
		if l.Name == "Ethernet" {
			t.Fatal("the unplugged LAN is still active")
		}
	}
	// and the other way round: the LAN is back
	setAdapters(lan, wifi1, wifi2)
	syncLinks(getAdapters(false))
	raw, _ = buildConfig(getSettings())
	validateConfig(t, raw, presentSet(lan, wifi1, wifi2))
}

func TestPresetsKeepConfigValidWithAnyLines(t *testing.T) {
	for _, mix := range [][]Adapter{{wifi1, wifi2}, {lan, wifi1, wifi2, mobile}} {
		for _, p := range builtinPresets() {
			freshSettings(t)
			setAdapters(mix...)
			syncLinks(mix)
			updateSettings(func(s *Settings) { applyPresetLocked(s, p) })
			raw, err := buildConfig(getSettings())
			if err != nil {
				t.Fatal(err)
			}
			validateConfig(t, raw, presentSet(mix...))
		}
	}
}

func TestKindDetection(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"Ethernet", "Intel(R) Ethernet Connection (2) I219-V", "ethernet"},
		{"Ethernet 2", "Realtek USB GbE Family Controller", "ethernet"},
		{"Wi-Fi", "Intel(R) Wi-Fi 6 AX201 160MHz", "wifi"},
		{"Wi-Fi 2", "TP-Link Wireless USB Adapter", "wifi"},
		{"Wi-Fi 3", "Realtek 8812BU Wireless LAN 802.11ac USB NIC", "wifi"},
		{"Wireless Network Connection", "Qualcomm Atheros QCA9377 Wireless Network Adapter", "wifi"},
		{"WLAN", "MediaTek Wi-Fi 6 MT7921 Wireless LAN Card", "wifi"},
		{"Ethernet 5", "Remote NDIS based Internet Sharing Device", "mobile"},
		{"Ethernet 3", "Apple Mobile Device Ethernet", "mobile"},
		{"Cellular", "Qualcomm Snapdragon X55 5G Mobile Broadband", "mobile"},
	}
	for _, c := range cases {
		if got := kindOf(c.name, c.desc); got != c.want {
			t.Errorf("%s / %s: %s, want %s", c.name, c.desc, got, c.want)
		}
		if isVirtual(c.name, c.desc) {
			t.Errorf("%s / %s is taken for a virtual adapter", c.name, c.desc)
		}
	}
}

func TestEverythingKeyedByNameNotKind(t *testing.T) {
	// capacity memory, probe state and quality are looked up by adapter name, so two Wi-Fi never share them
	freshSettings(t)
	probeMu.Lock()
	probes["Wi-Fi"] = probeState{dead: true}
	probeMu.Unlock()
	if !isDead("Wi-Fi") || isDead("Wi-Fi 2") {
		t.Fatal("state leaked between two Wi-Fi connections")
	}
}

func TestSpeedMemoryIsPerHardware(t *testing.T) {
	freshSettings(t)
	setAdapters(ad("Ethernet", "Intel(R) Ethernet Connection (2) I219-V", "192.168.1.10", 11))
	k1 := capKey("Ethernet")
	setAdapters(ad("Ethernet", "Realtek USB GbE Family Controller", "192.168.1.10", 11))
	if k2 := capKey("Ethernet"); k1 == k2 {
		t.Fatal("a different adapter with the same name shares the old speed memory")
	}
}

func TestUsageDoesNotJumpWhenTheAdapterBehindANameChanges(t *testing.T) {
	freshSettings(t)
	setAdapters(wifi1, wifi2)
	syncLinks(getAdapters(false))
	statsMu.Lock()
	tracks = map[string]*linkTrack{"Wi-Fi 2": {have: true, idx: 3, desc: "an older adapter", lastRx: 5_000_000, lastTx: 5_000_000, known: true, was: true}}
	statsMu.Unlock()
	lastSampleAt, lastCounterAt = time.Time{}, time.Time{}
	sample(false)
	statsMu.Lock()
	defer statsMu.Unlock()
	if tr := tracks["Wi-Fi 2"]; tr.totRx > 100_000_000 || tr.totTx > 100_000_000 {
		t.Fatalf("a false jump of %d bytes was counted", tr.totRx)
	}
}

func TestForgettingALineThatLeft(t *testing.T) {
	freshSettings(t)
	setAdapters(lan, wifi1)
	syncLinks(getAdapters(false))
	updateSettings(func(s *Settings) {
		s.Mode, s.VPNLink = "link:Ethernet", "Ethernet"
		s.AppRules = append(s.AppRules, AppRule{Process: "steam.exe", Target: "link:Ethernet"}, AppRule{Process: "b.exe", Target: "link:Wi-Fi"})
	})
	if err := forgetLink("Ethernet"); err == nil {
		t.Fatal("a connection that is plugged in was forgotten")
	}
	setAdapters(wifi1, wifi2) // the cable is gone, a second Wi-Fi came
	syncLinks(getAdapters(false))
	if err := forgetLink("Ethernet"); err != nil {
		t.Fatal(err)
	}
	s := getSettings()
	for _, l := range s.Links {
		if l.Name == "Ethernet" {
			t.Fatal("still listed")
		}
	}
	if s.Mode != "fast" || s.VPNLink != "" {
		t.Fatalf("settings still point at it: %q %q", s.Mode, s.VPNLink)
	}
	for _, r := range s.AppRules {
		if r.Process == "steam.exe" && r.Target != "fast" {
			t.Fatalf("rule still points at it: %s", r.Target)
		}
		if r.Process == "b.exe" && r.Target != "link:Wi-Fi" {
			t.Fatal("another rule was changed")
		}
	}
	if err := forgetLink("Nope"); err == nil {
		t.Fatal("unknown name accepted")
	}
	raw, err := buildConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	validateConfig(t, raw, presentSet(wifi1, wifi2))
}
