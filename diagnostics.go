package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// One-click diagnostics: a zip with the logs, the settings (without passwords or secrets) and a short summary,
// so a problem can be looked at by somebody else. Logs can contain website names, so the page tells the user to
// look before sending.

func tailBytes(path string, max int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	if st.Size() > max {
		_, _ = f.Seek(st.Size()-max, io.SeekStart)
	}
	b, _ := io.ReadAll(f)
	return b
}

func maskIP(ip string) string {
	if i := strings.LastIndex(ip, "."); i > 0 {
		return ip[:i] + ".x"
	}
	return ip
}

func sanitizedSettings() []byte {
	s := getSettings()
	s.Secret = ""
	if s.Share != nil {
		c := *s.Share
		c.Pass = ""
		s.Share = &c
	}
	s.DeviceNames = nil
	b, _ := json.MarshalIndent(s, "", "  ")
	return b
}

func diagnosticsSummary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "NetMerge %s\nTime: %s\nWindows/Go: %s/%s %s\n", appVersion, time.Now().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, runtime.Version())
	ver, _ := engineAlive()
	fmt.Fprintf(&sb, "Engine: %s (expected %s)\n\n", ver, engineVersion)
	snap := getSnapshot()
	fmt.Fprintf(&sb, "Merging: %v, connections: %d, down %.1f Mbps, up %.1f Mbps\n", snap.Running, snap.Conns, snap.TotalDown*8/1e6, snap.TotalUp*8/1e6)
	if snap.Error != "" {
		fmt.Fprintf(&sb, "Error: %s\n", snap.Error)
	}
	sb.WriteString("\nConnections:\n")
	for _, l := range snap.Links {
		hs := "unknown"
		if l.Healthy != nil {
			hs = fmt.Sprint(*l.Healthy)
		}
		fmt.Fprintf(&sb, "  %-12s present=%v enabled=%v healthy=%s delay=%dms ip=%s public=%s kind=%s rate=%s slots=%d quotaOut=%v\n",
			l.Label, l.Present, l.Enabled, hs, l.Delay, maskIP(l.IP), maskIP(l.PublicIP), l.Kind, l.LinkRate, l.Slots, l.QuotaOut)
	}
	if len(snap.SameISP) > 0 {
		fmt.Fprintf(&sb, "\nNote: %s show the same public address.\n", strings.Join(snap.SameISP, " and "))
	}
	return sb.String()
}

func registerDiagnosticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/diagnostics", guard(func(w http.ResponseWriter, r *http.Request) {
		name := "netmerge-diagnostics-" + time.Now().Format("20060102-1504") + ".zip"
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		zw := zip.NewWriter(w)
		add := func(n string, b []byte) {
			if len(b) == 0 {
				return
			}
			if f, err := zw.Create(n); err == nil {
				_, _ = f.Write(b)
			}
		}
		add("summary.txt", []byte(diagnosticsSummary()))
		add("settings.json", sanitizedSettings())
		d := filepath.Join(rootDir, "data")
		add("netmerge.log", tailBytes(filepath.Join(d, "netmerge.log"), 400<<10))
		add("engine.log", tailBytes(filepath.Join(d, "engine.log"), 200<<10))
		add("rates.log", tailBytes(filepath.Join(d, "rates.log"), 200<<10))
		add("go-crash.log", tailBytes(filepath.Join(d, "go-crash.log"), 100<<10))
		// what each line carried second by second over the last minutes (kept even without the debug log), and the
		// connections open right now with the line each one uses
		add("recent-rates.txt", recentRatesText())
		if b, code, err := apiReq("GET", "/connections", nil); err == nil && code == 200 {
			add("connections.json", b)
		}
		_ = zw.Close()
	}))
}
