package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// Backup: the settings that are worth moving to another PC or keeping before an experiment. Passwords, the engine
// secret and names given to phones are left out on purpose.
func registerBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/backup", guard(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="netmerge-settings-`+time.Now().Format("2006-01-02")+`.json"`)
		_, _ = w.Write(sanitizedSettings())
	}))
	mux.HandleFunc("/api/restore", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "POST only")
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeErr(w, 400, "Could not read the file")
			return
		}
		var in Settings
		if json.Unmarshal(b, &in) != nil || (in.Mode == "" && len(in.Links) == 0 && in.AppRules == nil) {
			writeErr(w, 400, "That does not look like a NetMerge settings file")
			return
		}
		updateSettings(func(s *Settings) { restoreInto(s, in) })
		// a backup from a version with the download manager may carry "downloadConfig"; it is ignored (Pullwell has
		// its own settings)
		if err := engine.Apply(); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
}

// restoreInto copies only the safe, user-facing fields. Connections are matched by adapter name.
func restoreInto(s *Settings, in Settings) {
	for i := range s.Links {
		for _, il := range in.Links {
			if il.Name == s.Links[i].Name {
				s.Links[i].Label, s.Links[i].Enabled, s.Links[i].Weight = il.Label, il.Enabled, il.Weight
				s.Links[i].QuotaGB, s.Links[i].Role = il.QuotaGB, il.Role
			}
		}
	}
	if in.Mode == "stable" || in.Mode == "fast" {
		s.Mode = in.Mode
	}
	if in.AppRules != nil {
		s.AppRules = in.AppRules
	}
	if in.BypassIPs != nil {
		s.BypassIPs = in.BypassIPs
	}
	if in.StableDomains != nil {
		s.StableDomains = in.StableDomains
	}
	s.MergeOnLaunch, s.BuiltinDNS = in.MergeOnLaunch, in.BuiltinDNS
	s.DNSProvider, s.DNSSecure, s.DNSCustom, s.DNSDomains = in.DNSProvider, in.DNSSecure, cleanIPs(in.DNSCustom), cleanDomains(in.DNSDomains)
	s.NoCallProtect, s.NoUpdateCheck, s.ClassicBalance = in.NoCallProtect, in.NoUpdateCheck, in.ClassicBalance
	if in.QuotaReset >= 1 && in.QuotaReset <= 28 {
		s.QuotaReset = in.QuotaReset
	}
	if in.QuotaAction == "warn" || in.QuotaAction == "leave" {
		s.QuotaAction = in.QuotaAction
	}
	for i := range s.AppRules {
		s.AppRules[i].Process = strings.TrimSpace(s.AppRules[i].Process)
	}
}
