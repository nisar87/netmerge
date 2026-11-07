package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Data used per connection, per day (rx+tx as counted by Windows on that adapter). Kept for ~2 months so a
// monthly plan can be watched; written once a minute and on quit.

var (
	usageMu     sync.Mutex
	usage       = map[string]map[string][2]uint64{} // day -> link -> rx,tx
	usageLoaded bool
	usageDirty  bool
)

func usagePath() string { return filepath.Join(rootDir, "data", "usage.json") }

func usageLoadLocked() {
	if usageLoaded {
		return
	}
	usageLoaded = true
	if b, err := os.ReadFile(usagePath()); err == nil {
		_ = json.Unmarshal(b, &usage)
	}
}

func usageAdd(link string, rx, tx uint64) {
	if rx == 0 && tx == 0 {
		return
	}
	usageMu.Lock()
	defer usageMu.Unlock()
	usageLoadLocked()
	day := time.Now().Format("2006-01-02")
	m := usage[day]
	if m == nil {
		m = map[string][2]uint64{}
		usage[day] = m
	}
	v := m[link]
	m[link] = [2]uint64{v[0] + rx, v[1] + tx}
	usageDirty = true
}

// periodStart is the first day of the current allowance period (the allowance renews on day `reset`).
func periodStart(now time.Time, reset int) time.Time {
	if reset < 1 || reset > 28 {
		reset = 1
	}
	y, m, d := now.Date()
	if d < reset {
		m--
	}
	return time.Date(y, m, reset, 0, 0, 0, 0, now.Location())
}

// usageSum returns bytes used today and in the current allowance period on a connection.
func usageSum(link string, reset int) (day, period uint64) {
	usageMu.Lock()
	defer usageMu.Unlock()
	usageLoadLocked()
	now := time.Now()
	d, from := now.Format("2006-01-02"), periodStart(now, reset).Format("2006-01-02")
	for k, m := range usage {
		if v, ok := m[link]; ok && k >= from {
			period += v[0] + v[1]
			if k == d {
				day = v[0] + v[1]
			}
		}
	}
	return
}

func usageRxTx(link string, reset int) (rx, tx uint64) {
	usageMu.Lock()
	defer usageMu.Unlock()
	usageLoadLocked()
	from := periodStart(time.Now(), reset).Format("2006-01-02")
	for k, m := range usage {
		if v, ok := m[link]; ok && k >= from {
			rx += v[0]
			tx += v[1]
		}
	}
	return
}

// ---- monthly allowance ----

var (
	quotaMu     sync.Mutex
	quotaOut    = map[string]bool{}
	quotaWarned = map[string]int{} // link|period start -> highest warning already shown (80 or 100)
)

// isQuotaOut: the connection has used its allowance and the user chose to leave it out of the merge then.
func isQuotaOut(name string) bool {
	quotaMu.Lock()
	defer quotaMu.Unlock()
	return quotaOut[name]
}

// quotaCheck compares each connection's use with its allowance: a warning at 80% and at 100%, and (if chosen)
// the connection is left out of the merge until the allowance renews or is raised.
func quotaCheck() {
	s := getSettings()
	start := periodStart(time.Now(), s.QuotaReset).Format("2006-01-02")
	out := map[string]bool{}
	for _, l := range s.Links {
		if l.QuotaGB <= 0 {
			continue
		}
		_, used := usageSum(l.Name, s.QuotaReset)
		limit := l.QuotaGB * 1e9
		pct := float64(used) / limit * 100
		key := l.Name + "|" + start
		quotaMu.Lock()
		warned := quotaWarned[key]
		level := 0
		if pct >= 100 {
			level = 100
		} else if pct >= 80 {
			level = 80
		}
		if level > warned {
			quotaWarned[key] = level
		}
		quotaMu.Unlock()
		if level > warned {
			if level == 100 {
				msg := fmt.Sprintf("%s has used its %.0f GB allowance.", l.Label, l.QuotaGB)
				if s.QuotaAction == "leave" {
					msg += " It is left out of the merge until the allowance renews."
				}
				trayBalloon("Allowance used up", msg)
				note("warn", "%s", msg)
				logf("quota: %s", msg)
			} else {
				note("warn", "%s has used %.0f%% of its allowance", l.Label, pct)
				trayBalloon("Allowance almost used", fmt.Sprintf("%s has used %.0f%% of its %.0f GB allowance.", l.Label, pct, l.QuotaGB))
				logf("quota: %s at %.0f%%", l.Label, pct)
			}
		}
		if level == 100 && s.QuotaAction == "leave" {
			out[l.Name] = true
		}
	}
	quotaMu.Lock()
	same := len(out) == len(quotaOut)
	for k := range out {
		if !quotaOut[k] {
			same = false
		}
	}
	quotaOut = out
	quotaMu.Unlock()
	if !same {
		logf("quota: connections left out of the merge: %v", out)
	}
}

type usageLinkOut struct {
	Name    string  `json:"name"`
	Label   string  `json:"label"`
	Enabled bool    `json:"enabled"`
	Quota   float64 `json:"quotaGB"`
	Used    uint64  `json:"used"`
	Rx      uint64  `json:"rx"`
	Tx      uint64  `json:"tx"`
	Out     bool    `json:"out"`
}

type usageDayOut struct {
	Date  string               `json:"date"`
	Links map[string][2]uint64 `json:"links"`
}

func registerUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/usage", guard(func(w http.ResponseWriter, r *http.Request) {
		s := getSettings()
		now := time.Now()
		start := periodStart(now, s.QuotaReset)
		var links []usageLinkOut
		for _, l := range s.Links {
			rx, tx := usageRxTx(l.Name, s.QuotaReset)
			links = append(links, usageLinkOut{l.Name, l.Label, l.Enabled, l.QuotaGB, rx + tx, rx, tx, isQuotaOut(l.Name)})
		}
		usageMu.Lock()
		usageLoadLocked()
		var days []usageDayOut
		for i := 0; i < 31; i++ {
			d := now.AddDate(0, 0, -i).Format("2006-01-02")
			days = append(days, usageDayOut{d, usage[d]})
		}
		usageMu.Unlock()
		writeJSON(w, map[string]any{
			"links": links, "days": days, "start": start.Format("2006-01-02"),
			"end": start.AddDate(0, 1, 0).Format("2006-01-02"), "reset": s.QuotaReset, "action": s.QuotaAction,
		})
	}))
}

func usageSave() {
	usageMu.Lock()
	defer usageMu.Unlock()
	if !usageDirty {
		return
	}
	var days []string
	for k := range usage {
		days = append(days, k)
	}
	sort.Strings(days)
	for len(days) > 62 {
		delete(usage, days[0])
		days = days[1:]
	}
	b, _ := json.Marshal(usage)
	tmp := usagePath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil && os.Rename(tmp, usagePath()) == nil {
		usageDirty = false
	}
}

func usageSaver() {
	time.Sleep(5 * time.Second)
	safely("quota check", quotaCheck)
	t := time.NewTicker(time.Minute)
	for range t.C {
		safely("usage save", usageSave)
		safely("quota check", quotaCheck)
	}
}
