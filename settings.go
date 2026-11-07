package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Link struct {
	Name    string  `json:"name"`    // Windows adapter alias, e.g. "Ethernet"
	Label   string  `json:"label"`   // display name
	Enabled bool    `json:"enabled"` // part of the merge
	Weight  int     `json:"weight"`  // 1..5, share of new connections (classic balancing)
	QuotaGB float64 `json:"quotaGB"` // monthly data allowance in GB (0 = no limit)
	Role    string  `json:"role"`    // "" (always used) | "busy" (only when the other lines are busy) | "backup" (only when the others are down)
}

type AppRule struct {
	Process string `json:"process"` // e.g. steam.exe
	Target  string `json:"target"`  // "fast" | "stable" | "link:<adapter name>" | "bypass" | "link:<adapter name>"
}

type Settings struct {
	Links          []Link            `json:"links"`
	Mode           string            `json:"mode"` // "fast" | "stable" | "link:<adapter name>"
	AppRules       []AppRule         `json:"appRules"`
	BypassIPs      []string          `json:"bypassIPs"`
	StableDomains  []string          `json:"stableDomains"`
	VPNLink        string            `json:"vpnLink"` // adapter that carries VPN tunnels ("" = first enabled)
	AutoStart      bool              `json:"autoStart"`
	MergeOnLaunch  bool              `json:"mergeOnLaunch"`
	BuiltinDNS     bool              `json:"builtinDNS"`
	SeenApps       []string          `json:"seenApps"`
	Secret         string            `json:"secret"`
	DebugLog       bool              `json:"debugLog"`
	TunStack       string            `json:"tunStack"`
	TunMTU         int               `json:"tunMTU"`
	Block          *BlockSettings    `json:"block"`
	StackChosen    bool              `json:"stackChosen"` // the user picked the engine mode themselves
	Share          *ShareSettings    `json:"share"`
	DeviceNames    map[string]string `json:"deviceNames,omitempty"` // names you gave to phones, keyed by hardware address
	QuotaReset     int               `json:"quotaReset"`            // day of the month the allowance renews (1-28)
	QuotaAction    string            `json:"quotaAction"`           // "warn" | "leave" (leave the connection out of the merge)
	SetupDone      bool              `json:"setupDone"`             // the first-run guide was finished or skipped
	NoCallProtect  bool              `json:"noCallProtect"`         // true = do not keep call apps (Teams, Zoom...) on one steady line
	NoUpdateCheck  bool              `json:"noUpdateCheck"`         // true = do not look for new versions by itself
	Preset         string            `json:"preset"`                // active preset id ("" = your own settings)
	PresetBackup   *PresetBackup     `json:"presetBackup,omitempty"`
	CustomPresets  []Preset          `json:"customPresets,omitempty"`
	PresetSchedule *PresetSchedule   `json:"presetSchedule,omitempty"`
	SchedActive    bool              `json:"schedActive"`
	SchedPrev      string            `json:"schedPrev"`
	ClassicBalance bool              `json:"classicBalance"` // true = old fixed round-robin instead of the smart dispatcher
}

var (
	settingsMu sync.Mutex
	settings   Settings
)

func defaultSettings() Settings {
	return Settings{
		Mode: "fast",
		AppRules: []AppRule{
			{Process: "mstsc.exe", Target: "stable"},
		},
		StableDomains: []string{
			"accounts.google.com", "login.live.com", "login.microsoftonline.com",
			"paypal.com", "appleid.apple.com", "whatsapp.net", "web.whatsapp.com",
		},
		MergeOnLaunch: true,
		Secret:        randomHex(16),
		Block:         defaultBlock(),
	}
}

func settingsPath() string { return filepath.Join(rootDir, "data", "settings.json") }

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = defaultSettings()
	b, err := os.ReadFile(settingsPath())
	if err == nil {
		if err := json.Unmarshal(b, &settings); err != nil {
			logf("settings.json unreadable (%v) - using defaults", err)
			settings = defaultSettings()
		}
	}
	if settings.Secret == "" {
		settings.Secret = randomHex(16)
	}
	normalizeLocked()
	if settings.Block == nil {
		settings.Block = defaultBlock()
	}
	// An old auto-tune left the engine on the slow user-space "gvisor" network stack (one download stream only
	// reached ~13-25 Mbps through it). Go back to the normal mode once; it can be changed again in Settings.
	if settings.TunStack == "gvisor" && !settings.StackChosen {
		settings.TunStack = ""
		logf("network engine mode reset from gvisor to the default (mixed)")
	}
	if settings.Mode != "stable" && !strings.HasPrefix(settings.Mode, "link:") {
		settings.Mode = "fast"
	}
	saveSettingsLocked()
}

func saveSettingsLocked() {
	_ = os.MkdirAll(filepath.Dir(settingsPath()), 0o755)
	b, _ := json.MarshalIndent(settings, "", "  ")
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err == nil {
		_ = os.Rename(tmp, settingsPath())
	}
}

func getSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	b, _ := json.Marshal(settings)
	var c Settings
	_ = json.Unmarshal(b, &c)
	return c
}

func updateSettings(fn func(s *Settings)) Settings {
	settingsMu.Lock()
	fn(&settings)
	normalizeLocked()
	saveSettingsLocked()
	settingsMu.Unlock()
	return getSettings()
}

func normalizeLocked() {
	for i := range settings.Links {
		if r := settings.Links[i].Role; r != "busy" && r != "backup" {
			settings.Links[i].Role = ""
		}
	}
	for k, v := range settings.DeviceNames {
		v = strings.TrimSpace(v)
		if len([]rune(v)) > 40 {
			v = string([]rune(v)[:40])
		}
		if v == "" {
			delete(settings.DeviceNames, k)
		} else {
			settings.DeviceNames[k] = v
		}
	}
	for i := range settings.Links {
		if settings.Links[i].Weight < 1 {
			settings.Links[i].Weight = 1
		}
		if settings.Links[i].Weight > 5 {
			settings.Links[i].Weight = 5
		}
		if settings.Links[i].QuotaGB < 0 || settings.Links[i].QuotaGB > 100000 {
			settings.Links[i].QuotaGB = 0
		}
		if strings.TrimSpace(settings.Links[i].Label) == "" {
			settings.Links[i].Label = settings.Links[i].Name
		}
	}
	if settings.QuotaReset < 1 || settings.QuotaReset > 28 {
		settings.QuotaReset = 1
	}
	if settings.QuotaAction != "leave" {
		settings.QuotaAction = "warn"
	}
	var rules []AppRule
	seen := map[string]bool{}
	for _, r := range settings.AppRules {
		p := strings.ToLower(strings.TrimSpace(r.Process))
		if p == "" || strings.ContainsAny(p, ",\"\n") || seen[p] {
			continue
		}
		if !strings.HasSuffix(p, ".exe") {
			p += ".exe"
		}
		seen[p] = true
		rules = append(rules, AppRule{Process: p, Target: r.Target})
	}
	settings.AppRules = rules
	settings.BypassIPs = cleanList(settings.BypassIPs, true)
	settings.StableDomains = cleanList(settings.StableDomains, false)
}

func cleanList(in []string, cidr bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.ToLower(strings.TrimSpace(v))
		v = strings.TrimPrefix(strings.TrimPrefix(v, "https://"), "http://")
		v = strings.TrimSuffix(v, "/")
		if v == "" || strings.ContainsAny(v, ", \"\n") || seen[v] {
			continue
		}
		if cidr && !strings.Contains(v, "/") {
			v += "/32"
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// syncLinks adds newly found adapters (enabled) and keeps settings for known ones.
func syncLinks(adapters []Adapter) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	changed := false
	for _, a := range adapters {
		found := false
		for _, l := range settings.Links {
			if l.Name == a.Name {
				found = true
				break
			}
		}
		if !found {
			label := a.Name
			if a.Kind == "wifi" {
				label = "Wi-Fi"
				if a.Name != "Wi-Fi" {
					label = a.Name
				}
			} else if a.Name == "Ethernet" {
				label = "LAN"
			}
			settings.Links = append(settings.Links, Link{Name: a.Name, Label: label, Enabled: true, Weight: 1})
			changed = true
			logf("new connection found: %s (%s)", a.Name, a.Desc)
			note("info", "New connection found: %s", label)
		}
	}
	if changed {
		normalizeLocked()
		saveSettingsLocked()
	}
}
