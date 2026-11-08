package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Presets: one click sets the mode, the call protection, a few app rules, ad blocking and the role of the mobile
// line. The settings from before are kept, so "Back to my settings" undoes it. Your own settings are the base:
// a preset only adds on top of them, so switching from one preset to another never piles rules up.

type Preset struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Desc    string    `json:"desc"`
	Builtin bool      `json:"builtin"`
	Mode    string    `json:"mode"`            // "fast" | "stable"
	NoCalls bool      `json:"noCalls"`         // switch call protection off
	Block   *bool     `json:"block,omitempty"` // ad blocking on/off; nil = leave as it is
	Mobile  string    `json:"mobile"`          // role for mobile lines: "" always | "busy" | "backup"
	Rules   []AppRule `json:"rules,omitempty"`
	// Downloads: what the download manager does while the preset is on: "" nothing | "pause" | "slow".
	Downloads string `json:"downloads,omitempty"`
}

type PresetBackup struct {
	Mode          string            `json:"mode"`
	NoCallProtect bool              `json:"noCallProtect"`
	AppRules      []AppRule         `json:"appRules"`
	BlockEnabled  bool              `json:"blockEnabled"`
	Roles         map[string]string `json:"roles"`
}

type PresetSchedule struct {
	Enabled bool   `json:"enabled"`
	From    string `json:"from"` // HH:MM
	To      string `json:"to"`
	Preset  string `json:"preset"`
}

func boolp(b bool) *bool { return &b }

func stableRules(names ...string) []AppRule {
	var r []AppRule
	for _, n := range names {
		r = append(r, AppRule{Process: n, Target: "stable"})
	}
	return r
}

func builtinPresets() []Preset {
	return []Preset{
		{ID: "work", Name: "Work from home", Builtin: true, Mode: "fast", Block: boolp(false), Mobile: "busy",
			Desc: "Meetings and remote desktop stay on one steady line. The mobile line joins only when the others are full. Ad blocking off so work sites do not break. Downloads slow down so meetings stay smooth.", Downloads: "slow",
			Rules: stableRules("mstsc.exe", "AnyDesk.exe", "TeamViewer.exe", "Citrix.exe", "wfica32.exe", "Slack.exe", "Teams.exe", "ms-teams.exe", "Zoom.exe", "webex.exe")},
		{ID: "gaming", Name: "Gaming", Builtin: true, Mode: "fast", Mobile: "busy",
			Desc: "Games keep each server on one line so the connection stays steady. Voice chat is protected. The mobile line joins only when the others are full. Downloads wait until you stop playing.", Downloads: "pause",
			Rules: stableRules("VALORANT-Win64-Shipping.exe", "RiotClientServices.exe", "LeagueClient.exe", "League of Legends.exe", "cs2.exe", "csgo.exe", "dota2.exe", "r5apex.exe",
				"FortniteClient-Win64-Shipping.exe", "GTA5.exe", "Overwatch.exe", "TslGame.exe", "RocketLeague.exe", "Discord.exe")},
		{ID: "maxspeed", Name: "Max speed", Builtin: true, Mode: "fast", NoCalls: true,
			Desc: "Every line used all the time and call protection off. For big downloads when nobody is on a call. Downloads run at full speed."},
		{ID: "travel", Name: "Travel / limited data", Builtin: true, Mode: "fast", Mobile: "backup",
			Desc: "The mobile line is used only if the other lines fail, so your data allowance lasts."},
		{ID: "safe", Name: "Safe browsing", Builtin: true, Mode: "stable", Block: boolp(true),
			Desc: "Each website stays on one line (fewer sign-in problems) and ad and tracker blocking is on."},
	}
}

func allPresets(s Settings) []Preset {
	return append(builtinPresets(), s.CustomPresets...)
}

func findPreset(s Settings, id string) (Preset, bool) {
	for _, p := range allPresets(s) {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

func snapshotLocked(s *Settings) *PresetBackup {
	b := &PresetBackup{Mode: s.Mode, NoCallProtect: s.NoCallProtect, AppRules: append([]AppRule(nil), s.AppRules...), Roles: map[string]string{}}
	b.BlockEnabled = s.Block != nil && s.Block.Enabled
	for _, l := range s.Links {
		b.Roles[l.Name] = l.Role
	}
	return b
}

func restoreLocked(s *Settings, b *PresetBackup) {
	s.Mode, s.NoCallProtect, s.AppRules = b.Mode, b.NoCallProtect, append([]AppRule(nil), b.AppRules...)
	if s.Block == nil {
		s.Block = defaultBlock()
	}
	nb := *s.Block
	nb.Enabled = b.BlockEnabled
	s.Block = &nb
	for i := range s.Links {
		if r, ok := b.Roles[s.Links[i].Name]; ok {
			s.Links[i].Role = r
		}
	}
}

func applyPresetLocked(s *Settings, p Preset) {
	if s.PresetBackup == nil {
		s.PresetBackup = snapshotLocked(s)
	}
	base := *s.PresetBackup
	restoreLocked(s, &base) // start from the user's own settings
	if p.Mode == "stable" || p.Mode == "fast" {
		if !strings.HasPrefix(s.Mode, "link:") || p.Mode == "stable" {
			s.Mode = p.Mode
		}
	}
	s.NoCallProtect = p.NoCalls
	if p.Block != nil {
		if s.Block == nil {
			s.Block = defaultBlock()
		}
		nb := *s.Block
		nb.Enabled = *p.Block
		s.Block = &nb
	}
	have := map[string]bool{}
	for _, r := range s.AppRules {
		have[strings.ToLower(r.Process)] = true
	}
	for _, r := range p.Rules {
		if !have[strings.ToLower(r.Process)] { // your own rule for a program always wins
			s.AppRules = append(s.AppRules, r)
		}
	}
	mobile := map[string]bool{}
	for _, a := range getAdapters(false) {
		if a.Kind == "mobile" {
			mobile[a.Name] = true
		}
	}
	for i := range s.Links {
		if mobile[s.Links[i].Name] {
			s.Links[i].Role = p.Mobile
		}
	}
	s.Preset = p.ID
}

func presetApply(id string) error {
	s := getSettings()
	p, ok := findPreset(s, id)
	if !ok {
		return nil
	}
	updateSettings(func(s *Settings) { applyPresetLocked(s, p) })
	note("info", "Preset \"%s\" is on", p.Name)
	return engine.Apply()
}

func presetRestore() error {
	updateSettings(func(s *Settings) {
		if s.PresetBackup != nil {
			restoreLocked(s, s.PresetBackup)
		}
		s.PresetBackup, s.Preset = nil, ""
	})
	note("info", "Back to your own settings")
	return engine.Apply()
}

func inWindow(now time.Time, from, to string) bool {
	f, e1 := time.Parse("15:04", from)
	t, e2 := time.Parse("15:04", to)
	if e1 != nil || e2 != nil {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	fm, tm := f.Hour()*60+f.Minute(), t.Hour()*60+t.Minute()
	if fm == tm {
		return false
	}
	if fm < tm {
		return m >= fm && m < tm
	}
	return m >= fm || m < tm // over midnight
}

// presetScheduleLoop switches to the scheduled preset inside its time window and back afterwards.
func presetScheduleLoop() {
	t := time.NewTicker(30 * time.Second)
	for range t.C {
		safely("preset schedule", func() {
			s := getSettings()
			sc := s.PresetSchedule
			if sc == nil || !sc.Enabled {
				return
			}
			in := inWindow(time.Now(), sc.From, sc.To)
			switch {
			case in && !s.SchedActive:
				if _, ok := findPreset(s, sc.Preset); !ok {
					return
				}
				updateSettings(func(s *Settings) { s.SchedActive, s.SchedPrev = true, s.Preset })
				_ = presetApply(sc.Preset)
			case !in && s.SchedActive:
				prev := s.SchedPrev
				updateSettings(func(s *Settings) { s.SchedActive, s.SchedPrev = false, "" })
				if prev == "" {
					_ = presetRestore()
				} else {
					_ = presetApply(prev)
				}
			}
		})
	}
}

func registerPresetRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/presets", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in struct {
				Action   string          `json:"action"`
				ID       string          `json:"id"`
				Name     string          `json:"name"`
				Schedule *PresetSchedule `json:"schedule"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
				writeErr(w, 400, "bad request")
				return
			}
			var err error
			switch in.Action {
			case "apply":
				if _, ok := findPreset(getSettings(), in.ID); !ok {
					writeErr(w, 404, "unknown preset")
					return
				}
				err = presetApply(in.ID)
			case "restore":
				err = presetRestore()
			case "save":
				name := strings.TrimSpace(in.Name)
				if name == "" || len(name) > 40 {
					writeErr(w, 400, "Give the preset a name (up to 40 letters)")
					return
				}
				updateSettings(func(s *Settings) {
					p := Preset{ID: "my-" + randomHex(4), Name: name, Desc: "Your own preset.", Mode: s.Mode, NoCalls: s.NoCallProtect, Rules: append([]AppRule(nil), s.AppRules...)}
					if s.Block != nil {
						p.Block = boolp(s.Block.Enabled)
					}
					if len(s.CustomPresets) < 12 {
						s.CustomPresets = append(s.CustomPresets, p)
					}
				})
			case "delete":
				updateSettings(func(s *Settings) {
					var keep []Preset
					for _, p := range s.CustomPresets {
						if p.ID != in.ID {
							keep = append(keep, p)
						}
					}
					s.CustomPresets = keep
					if s.Preset == in.ID {
						s.Preset = ""
					}
				})
			case "schedule":
				sc := in.Schedule
				if sc != nil {
					if _, e1 := time.Parse("15:04", sc.From); e1 != nil {
						writeErr(w, 400, "Start time must look like 22:00")
						return
					}
					if _, e2 := time.Parse("15:04", sc.To); e2 != nil {
						writeErr(w, 400, "End time must look like 06:00")
						return
					}
					if _, ok := findPreset(getSettings(), sc.Preset); !ok {
						writeErr(w, 400, "Choose a preset")
						return
					}
				}
				updateSettings(func(s *Settings) { s.PresetSchedule = sc })
			default:
				writeErr(w, 400, "unknown action")
				return
			}
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		s := getSettings()
		writeJSON(w, map[string]any{"presets": allPresets(s), "active": s.Preset, "schedule": s.PresetSchedule, "scheduled": s.SchedActive})
	}))
}
