package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// set once the user has used the power switch, so start-up retries never fight their choice
var manualPower atomic.Bool

//go:embed ui
var uiFS embed.FS

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": msg})
}

// Only our own page may talk to the API: the Host header must be local (blocks DNS-rebinding) and
// changes need the custom header plus a local Origin (blocks other websites from driving the app).
func guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1:") && !strings.HasPrefix(o, "http://localhost:") {
			writeErr(w, 403, "forbidden")
			return
		}
		if r.Method != "GET" && r.Header.Get("X-NetMerge") != "1" {
			writeErr(w, 403, "forbidden")
			return
		}
		h(w, r)
	}
}

func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := strings.ToLower(r.Host)
		if h != "127.0.0.1:9196" && h != "localhost:9196" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var quitCh = make(chan struct{})
var quitOnce sync.Once

func requestQuit() { quitOnce.Do(func() { close(quitCh) }) }

func routes() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(uiFS, "ui")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/api/state", guard(func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		s.Secret = ""
		writeJSON(w, map[string]any{"snap": getSnapshot(), "settings": s, "test": getTest(), "adapters": getAdapters(false), "block": blockSummary(), "conflict": detectConflict()})
	}))

	mux.HandleFunc("/api/power", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			On bool `json:"on"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		manualPower.Store(true)
		if req.On {
			if err := engine.Start(); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		} else {
			engine.Stop()
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/settings", guard(func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, 400, "bad settings")
			return
		}
		old := getSettings()
		// Take only what the page is allowed to change, and merge links by name, so a page that is a
		// moment out of date can never wipe newer values (new adapters, shares set by a speed test).
		ns := updateSettings(func(s *Settings) {
			for i := range s.Links {
				for _, il := range in.Links {
					if il.Name == s.Links[i].Name {
						s.Links[i].Label, s.Links[i].Enabled, s.Links[i].Weight = il.Label, il.Enabled, il.Weight
						s.Links[i].QuotaGB = il.QuotaGB
						s.Links[i].Role = il.Role
					}
				}
			}
			s.Mode = "fast"
			if in.Mode == "stable" {
				s.Mode = "stable"
			} else if strings.HasPrefix(in.Mode, "link:") {
				for _, l := range s.Links {
					if in.Mode == "link:"+l.Name {
						s.Mode = in.Mode
					}
				}
			}
			s.AppRules, s.BypassIPs, s.StableDomains = in.AppRules, in.BypassIPs, in.StableDomains
			s.VPNLink, s.AutoStart, s.MergeOnLaunch = in.VPNLink, in.AutoStart, in.MergeOnLaunch
			s.BuiltinDNS, s.DebugLog = in.BuiltinDNS, in.DebugLog
			s.ClassicBalance = in.ClassicBalance
			s.NoUpdateCheck = in.NoUpdateCheck
			s.NoCallProtect = in.NoCallProtect
			s.SetupDone = s.SetupDone || in.SetupDone
			s.QuotaReset, s.QuotaAction = in.QuotaReset, in.QuotaAction
			if in.TunStack != s.TunStack {
				switch in.TunStack {
				case "", "mixed", "system", "gvisor":
					s.TunStack, s.StackChosen = in.TunStack, true
				}
			}
		})
		go safely("quota check", quotaCheck)
		if ns.AutoStart != old.AutoStart {
			if err := setAutoStart(ns.AutoStart); err != nil {
				logf("autostart: %v", err)
				updateSettings(func(s *Settings) { s.AutoStart = old.AutoStart })
				writeErr(w, 500, "Could not change start-with-Windows: "+err.Error())
				return
			}
		}
		if ns.TunStack != old.TunStack {
			// the virtual adapter has to be rebuilt for a different network mode
			if running, _, _, _ := engine.Status(); running {
				engine.Stop()
				time.Sleep(time.Second)
				if err := engine.Start(); err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			}
		}
		if err := engine.Apply(); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	registerBlockRoutes(mux)
	registerUsageRoutes(mux)
	registerShareRoutes(mux)
	registerUpdateRoutes(mux)
	registerDiagnosticsRoutes(mux)
	registerHistoryRoutes(mux)
	registerBackupRoutes(mux)
	registerPresetRoutes(mux)
	mux.HandleFunc("/api/events", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"events": allEvents()})
	}))

	mux.HandleFunc("/api/rescan", guard(func(w http.ResponseWriter, r *http.Request) {
		syncLinks(getAdapters(true))
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/speedtest", guard(func(w http.ResponseWriter, r *http.Request) {
		go safely("speed test", runSpeedTest)
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/applyweights", guard(func(w http.ResponseWriter, r *http.Request) {
		t := getTest()
		updateSettings(func(s *Settings) {
			for i := range s.Links {
				if v, ok := t.Suggest[s.Links[i].Name]; ok {
					s.Links[i].Weight = v
				}
			}
		})
		if err := engine.Apply(); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/log", guard(func(w http.ResponseWriter, r *http.Request) {
		name := "netmerge.log"
		switch r.URL.Query().Get("f") {
		case "engine":
			name = "engine.log"
		case "rates":
			name = "rates.log"
		}
		writeJSON(w, map[string]string{"text": tailFile(filepath.Join(rootDir, "data", name), 300)})
	}))

	mux.HandleFunc("/api/quit", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); requestQuit() }()
	}))

	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]int{"pid": os.Getpid()}) })
	return hostGuard(mux)
}
