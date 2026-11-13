package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	nmengine "netmerge/engine"
)

// Every kind of settings file NetMerge can write must be understood by its engine: each setting is switched on in
// turn, the file NetMerge writes is loaded by the engine, and every key in it must be one the engine reads
// (or one listed in notNeeded, with the reason).
func TestEngineUnderstandsEverySetting(t *testing.T) {
	notNeeded := map[string]string{} // keys NetMerge writes that the engine may ignore (none at the moment)
	known := map[string]bool{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		if a, ok := v.([]any); ok { // lists of sections (proxies, groups, listeners): their keys under the list's name
			for _, x := range a {
				walk(prefix, x)
			}
			return
		}
		if m, ok := v.(map[string]any); ok {
			for k, x := range m {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				known[p] = true
				walk(p, x)
			}
		}
	}

	rootDir = t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	adapterCache, adapterAt = []Adapter{{Name: "lo", Kind: "ethernet", Up: true}, {Name: "lo2", Kind: "wifi", Up: true}}, time.Now().Add(time.Hour)
	t.Cleanup(func() { adapterCache, adapterAt = nil, time.Time{} })
	base := func() Settings {
		settings = defaultSettings()
		s := getSettings()
		s.Links = []Link{{Name: "lo", Label: "Cable", Enabled: true, Weight: 1}, {Name: "lo2", Label: "Wi-Fi", Enabled: true, Weight: 2, Role: "busy"}}
		s.Secret = "compat"
		return s
	}
	variants := map[string]func(*Settings){
		"defaults":      func(s *Settings) {},
		"stable mode":   func(s *Settings) { s.Mode = "stable" },
		"one line only": func(s *Settings) { s.Mode = "link:lo2" },
		"app rules": func(s *Settings) {
			s.AppRules = []AppRule{{"steam.exe", "fast"}, {"chrome.exe", "stable"}, {"vpn.exe", "bypass"}, {"game.exe", "link:lo"}}
		},
		"routing exceptions": func(s *Settings) {
			s.BypassIPs = []string{"203.0.113.0/24", "198.51.100.7"}
			s.StableDomains = []string{"bank.example"}
		},
		"VPN line": func(s *Settings) { s.VPNLink = "lo2" },
		"blocking": func(s *Settings) { s.Block = defaultBlock(); s.Block.Enabled = true },
		"sharing": func(s *Settings) {
			s.Share = &ShareSettings{Enabled: true, Port: 17931, SSPort: 17932, User: "u", Pass: "p"}
		},
		"built-in DNS": func(s *Settings) {
			s.BuiltinDNS, s.DNSProvider, s.DNSSecure, s.DNSDomains = true, "opendns", true, []string{"corp.example"}
		},
		"custom DNS":         func(s *Settings) { s.BuiltinDNS, s.DNSProvider, s.DNSCustom = true, "custom", []string{"9.9.9.9"} },
		"MTU":                func(s *Settings) { s.TunMTU = 1400 },
		"debug log":          func(s *Settings) { s.DebugLog = true },
		"no call protection": func(s *Settings) { s.NoCallProtect = true },
	}
	names := make([]string, 0, len(variants))
	for n := range variants {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			s := base()
			variants[name](&s)
			settings = s
			if blockCfg(s).Enabled {
				os.MkdirAll(filepath.Join(configDir(), "filters"), 0o755)
				os.WriteFile(filepath.Join(configDir(), "filters", "nm_block.txt"), []byte("+.ads.example\n"), 0o644)
				os.WriteFile(filepath.Join(configDir(), "filters", "nm_allow.txt"), []byte("+.ok.example\n"), 0o644)
			}
			b, err := buildConfig(s)
			if err != nil {
				t.Fatal(err)
			}
			var cfg map[string]any
			json.Unmarshal(b, &cfg)
			// keys the engine reads, from its own settings structure
			engineKeys := map[string]bool{}
			for _, k := range nmengine.ConfigKeys() {
				engineKeys[k] = true
			}
			known = map[string]bool{}
			walk("", cfg)
			for k := range known {
				if strings.HasPrefix(k, "sniffer.sniff.") || strings.HasPrefix(k, "dns.nameserver-policy.") || strings.HasPrefix(k, "rule-providers.") {
					continue // names chosen by NetMerge, not settings
				}
				if !engineKeys[k] && notNeeded[k] == "" {
					t.Errorf("NetMerge writes %q, which the engine does not read", k)
				}
			}
			// load it for real (without capturing traffic, which needs administrator rights)
			cfg["tun"].(map[string]any)["enable"] = false
			cfg["external-controller"] = "127.0.0.1:0"
			out, _ := json.Marshal(cfg)
			os.MkdirAll(configDir(), 0o755)
			path := filepath.Join(configDir(), "config.yaml")
			os.WriteFile(path, append([]byte(configHeader), out...), 0o644)
			e := nmengine.New(configDir(), io.Discard)
			if err := e.Start(path); err != nil {
				t.Fatalf("the engine refused this setting: %v", err)
			}
			e.Stop()
		})
	}
}
