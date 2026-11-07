package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const histLen = 90

type LinkStat struct {
	Name     string    `json:"name"`
	Label    string    `json:"label"`
	Present  bool      `json:"present"`
	Enabled  bool      `json:"enabled"`
	Weight   int       `json:"weight"`
	Kind     string    `json:"kind"`
	Desc     string    `json:"desc"`
	IP       string    `json:"ip"`
	LinkRate string    `json:"linkRate"`
	Down     float64   `json:"down"` // bytes/s
	Up       float64   `json:"up"`
	Healthy  *bool     `json:"healthy"`
	Delay    int       `json:"delay"`
	Conns    int       `json:"conns"`
	History  []float64 `json:"history"`
	TotalRx  uint64    `json:"totalRx"`
	TotalTx  uint64    `json:"totalTx"`
	PublicIP string    `json:"publicIP"`
	Slots    int       `json:"slots"`    // of dispatchSlots: share of new connections the smart dispatcher gives it
	UseDay   uint64    `json:"useDay"`   // bytes today
	UseMonth uint64    `json:"useMonth"` // bytes in the current allowance period
	QuotaOut bool      `json:"quotaOut"` // allowance used up and the connection is left out
	Role     string    `json:"role"`
	Standby  bool      `json:"standby"` // role says it is not needed right now
	Slow     bool      `json:"slow"`    // far slower than the best line: kept as standby, no new connections
	Quality  Quality   `json:"quality"`
	Portal   bool      `json:"portal"` // the network wants a sign-in page first (hotel / café Wi-Fi)
}

// DeviceStat is another device (a phone) whose traffic arrives through proxy sharing.
type DeviceStat struct {
	IP    string  `json:"ip"`
	Conns int     `json:"conns"`
	Down  float64 `json:"down"`
	Up    float64 `json:"up"`
}

type AppStat struct {
	Process string         `json:"process"`
	Down    float64        `json:"down"`
	Up      float64        `json:"up"`
	Conns   int            `json:"conns"`
	Links   map[string]int `json:"links"`
	Rule    string         `json:"rule"`
}

type Snapshot struct {
	Running   bool            `json:"running"`
	Version   string          `json:"version"`
	App       string          `json:"app"` // NetMerge version (the window reloads itself when it changes)
	Error     string          `json:"error"`
	Uptime    int64           `json:"uptime"`
	Links     []LinkStat      `json:"links"`
	Apps      []AppStat       `json:"apps"`
	TotalDown float64         `json:"totalDown"`
	TotalUp   float64         `json:"totalUp"`
	PeakDown  float64         `json:"peakDown"`
	Conns     int             `json:"conns"`
	Total     []float64       `json:"total"`
	Devices   []DeviceStat    `json:"devices"` // other devices using proxy sharing
	Smart     bool            `json:"smart"`   // the smart dispatcher is steering new connections
	Activity  []ActivityEvent `json:"activity"`
	SameISP   []string        `json:"sameIsp"` // labels of lines that show the same public address (probably one provider)
}

type linkTrack struct {
	lastRx, lastTx uint32
	have           bool
	rx, tx         float64
	hist           []float64
	totRx, totTx   uint64
}

type connTrack struct{ down, up int64 }

var (
	statsMu   sync.Mutex
	tracks    = map[string]*linkTrack{}
	conns     = map[string]connTrack{}
	snap      Snapshot
	totalHist []float64
	peak      float64
	health    = map[string]linkHealth{}
	publicIPs = map[string]string{}
)

// linkHealth is the last engine health-check result for a link; fails counts consecutive bad readings.
type linkHealth struct {
	ok    bool
	delay int
	fails int
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > histLen {
		h = h[len(h)-histLen:]
	}
	return h
}

func statsLoop() {
	t := time.NewTicker(time.Second)
	n, changed := 0, 0
	var last time.Time
	for range t.C {
		if now := time.Now(); resumeDetected(last, now) {
			onResume()
		}
		last = time.Now()
		safely("sampler", func() { sample(n%5 == 0) })
		if n%5 == 0 {
			safely("engine check", func() {
				engine.Check()
				// a connection was plugged in / removed / re-weighted: reload the engine (after it stays that way 10 s)
				if running, _, _, _ := engine.Status(); running && engine.AppliedKey() != activeKey() {
					changed++
					if changed >= 2 {
						changed = 0
						logf("connections changed - updating the engine")
						go safely("apply", func() { _ = engine.Apply() })
					}
				} else {
					changed = 0
				}
			})
		}
		n++
	}
}

type connList struct {
	Connections []mConn `json:"connections"`
}

var (
	lastApps      map[string]*AppStat
	lastDevs      map[string]*DeviceStat
	lastLinkConns map[string]int
	lastConnN     int
)

type connResult struct {
	ok   bool
	at   time.Time
	list connList
}

var connPending chan connResult

type mConn struct {
	ID       string   `json:"id"`
	Download int64    `json:"download"`
	Upload   int64    `json:"upload"`
	Chains   []string `json:"chains"`
	Rule     string   `json:"rule"`
	Payload  string   `json:"rulePayload"`
	Metadata struct {
		Process  string `json:"process"`
		Type     string `json:"type"`
		SrcIP    string `json:"sourceIP"`
		DestIP   string `json:"destinationIP"`
		DestPort string `json:"destinationPort"`
		Host     string `json:"host"`
	} `json:"metadata"`
}

var lastSampleAt, lastCounterAt, lastConnAt time.Time

func sample(slow bool) {
	now := time.Now()
	dtSec := 1.0
	if !lastSampleAt.IsZero() {
		dtSec = now.Sub(lastSampleAt).Seconds()
		if dtSec < 0.5 { // timer backlog after a slow tick: skip, never divide by a tiny interval
			return
		}
	}
	lastSampleAt = now
	s := getSettings()
	adapters := getAdapters(false)
	byName := map[string]Adapter{}
	for _, a := range adapters {
		byName[a.Name] = a
	}
	if slow {
		syncLinks(adapters)
		s = getSettings()
	}
	counters := ifCounters()
	// the byte counters were read just now: their interval is measured between reads, not from the start of the
	// sample (a slow engine reply in between used to stretch one interval and shrink the next: false 200+ Mbps spikes)
	cNow := time.Now()
	dtC := 1.0
	if !lastCounterAt.IsZero() {
		dtC = cNow.Sub(lastCounterAt).Seconds()
	}
	if dtC > 2.2 {
		logf("sampler was held up for %.1f s", dtC)
	}
	lastCounterAt = cNow
	running, errMsg, started, version := engine.Status()

	// --- engine connections (per app / per link) ---
	var mc connList
	// The engine's connection list can take seconds when the engine is under load. It is fetched in the background:
	// if it is not back within a moment, this tick keeps the previous per-app numbers and only the link speeds
	// (read from the adapters, always instant) move, so the window never freezes.
	gotConns := false
	var connAt time.Time
	if running {
		if connPending == nil {
			ch := make(chan connResult, 1)
			connPending = ch
			go func() {
				var r connResult
				if b, code, err := apiReq("GET", "/connections", nil); err == nil && code == 200 {
					_ = json.Unmarshal(b, &r.list)
					r.ok = true
				}
				r.at = time.Now()
				ch <- r
			}()
		}
		var r connResult
		select {
		case r = <-connPending:
			connPending = nil
		case <-time.After(300 * time.Millisecond):
			r.ok = false
		}
		if r.ok {
			gotConns, connAt = true, r.at
			mc.Connections = r.list.Connections
		}
	} else {
		connPending = nil
	}

	connNow := connAt
	if !gotConns {
		connNow = time.Now()
	}
	dtConn := dtSec
	if !lastConnAt.IsZero() {
		dtConn = connNow.Sub(lastConnAt).Seconds()
	}
	if gotConns {
		lastConnAt = connNow
	}
	if dtConn < 0.2 {
		dtConn = 0.2
	}

	statsMu.Lock()
	defer statsMu.Unlock()

	type dbgConn struct {
		link string
		down float64
		dst  string
	}
	var dbgRows []dbgConn
	devs := map[string]*DeviceStat{}
	apps := map[string]*AppStat{}
	linkConns := map[string]int{}
	newConns := map[string]connTrack{}
	for _, c := range mc.Connections {
		proc := strings.ToLower(c.Metadata.Process)
		if proc == "" {
			proc = "system"
		}
		prev, had := conns[c.ID]
		d, u := float64(0), float64(0)
		if had {
			d, u = float64(c.Download-prev.down)/dtConn, float64(c.Upload-prev.up)/dtConn
			if d < 0 {
				d = 0
			}
			if u < 0 {
				u = 0
			}
		}
		newConns[c.ID] = connTrack{c.Download, c.Upload}
		a := apps[proc]
		if a == nil {
			a = &AppStat{Process: proc, Links: map[string]int{}}
			apps[proc] = a
		}
		a.Down += d
		a.Up += u
		a.Conns++
		// connections that arrive from other devices (phones using proxy sharing) rather than from this PC
		if t := c.Metadata.Type; t != "" && t != "Tun" && t != "Inner" && c.Metadata.SrcIP != "" && c.Metadata.SrcIP != "127.0.0.1" {
			dv := devs[c.Metadata.SrcIP]
			if dv == nil {
				dv = &DeviceStat{IP: c.Metadata.SrcIP}
				devs[c.Metadata.SrcIP] = dv
			}
			dv.Conns++
			dv.Down += d
			dv.Up += u
		}
		ln := ""
		if len(c.Chains) > 0 {
			ln = proxyToLink(s, c.Chains[0])
			if ln == "" && c.Chains[0] == "DIRECT" {
				ln = "bypass"
			}
		}
		if ln != "" {
			a.Links[ln]++
			linkConns[ln]++
		}
		if s.DebugLog && d > 12500 {
			dst := c.Metadata.Host
			if dst == "" {
				dst = c.Metadata.DestIP
			}
			dbgRows = append(dbgRows, dbgConn{labelFor(s, ln), d, dst + ":" + c.Metadata.DestPort})
		}
	}
	if gotConns {
		conns = newConns
		lastApps, lastDevs, lastLinkConns, lastConnN = apps, devs, linkConns, len(mc.Connections)
	} else if running {
		apps, devs, linkConns = lastApps, lastDevs, lastLinkConns
		if apps == nil {
			apps, devs, linkConns = map[string]*AppStat{}, map[string]*DeviceStat{}, map[string]int{}
		}
		mc.Connections = make([]mConn, lastConnN)
	} else {
		conns = newConns
		lastApps, lastDevs, lastLinkConns, lastConnN = nil, nil, nil, 0
	}

	// remember apps we have seen, for the app picker
	if len(apps) > 0 {
		go rememberApps(apps)
	}

	var links []LinkStat
	var totDown, totUp float64
	for _, l := range s.Links {
		tr := tracks[l.Name]
		if tr == nil {
			tr = &linkTrack{}
			tracks[l.Name] = tr
		}
		a, present := byName[l.Name]
		if present {
			if c, ok := counters[a.Index]; ok {
				if tr.have {
					dr, dtx := c[0]-tr.lastRx, c[1]-tr.lastTx // uint32 wrap-safe
					// A reading that comes right after a stall (no sample for >2.2 s) or right behind another one
					// (<0.5 s) is not trusted for the speed shown: the previous speed is kept for that tick. The
					// bytes still count towards the totals, so usage stays exact.
					if dtC >= 0.5 && dtC <= 2.2 {
						tr.rx, tr.tx = float64(dr)/dtC, float64(dtx)/dtC
					}
					tr.totRx += uint64(dr)
					tr.totTx += uint64(dtx)
					usageAdd(l.Name, uint64(dr), uint64(dtx))
				}
				tr.lastRx, tr.lastTx, tr.have = c[0], c[1], true
			}
		} else {
			tr.have, tr.rx, tr.tx = false, 0, 0
		}
		tr.hist = push(tr.hist, tr.rx)
		ls := LinkStat{Name: l.Name, Label: l.Label, Present: present, Enabled: l.Enabled, Weight: l.Weight,
			Kind: a.Kind, Desc: a.Desc, IP: a.IP, LinkRate: a.LinkRate, Down: tr.rx, Up: tr.tx,
			Conns: linkConns[l.Name], History: append([]float64(nil), tr.hist...), TotalRx: tr.totRx, TotalTx: tr.totTx,
			PublicIP: publicIPs[l.Name]}
		ls.UseDay, ls.UseMonth = usageSum(l.Name, s.QuotaReset)
		ls.QuotaOut = isQuotaOut(l.Name)
		ls.Quality = qualityFor(l.Name)
		ls.Portal = isPortal(l.Name)
		ls.Role, ls.Standby = l.Role, running && l.Enabled && present && isStandby(l)
		if n, on := dispatchSlotsOf(l.Name); on && n == 0 && running && l.Enabled && present && !ls.Standby {
			ls.Slow = true
		}
		if n, ok := dispatchSlotsOf(l.Name); ok {
			ls.Slots = n
		}
		if h, ok := health[l.Name]; ok && running && l.Enabled && present {
			// Only call a link offline after several bad readings in a row AND no traffic on it:
			// a link that is busy downloading obviously works, even if a health probe was slow.
			v := !(h.fails >= 3 && tr.rx < 12500 && tr.tx < 12500)
			ls.Healthy, ls.Delay = &v, h.delay
		}
		if present {
			totDown += tr.rx
			totUp += tr.tx
		}
		links = append(links, ls)
	}
	totalHist = push(totalHist, totDown)
	if totDown > peak {
		peak = totDown
	}

	ruleOf := map[string]string{}
	for _, r := range s.AppRules {
		ruleOf[r.Process] = r.Target
	}
	var appList []AppStat
	for _, a := range apps {
		a.Rule = ruleOf[a.Process]
		appList = append(appList, *a)
	}
	sort.Slice(appList, func(i, j int) bool {
		if appList[i].Down+appList[i].Up != appList[j].Down+appList[j].Up {
			return appList[i].Down+appList[i].Up > appList[j].Down+appList[j].Up
		}
		return appList[i].Conns > appList[j].Conns
	})
	if len(appList) > 40 {
		appList = appList[:40]
	}

	if s.DebugLog && (totDown > 62500 || totUp > 62500) {
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s total dn=%.1f up=%.1f Mbps", time.Now().Format("15:04:05"), totDown*8/1e6, totUp*8/1e6)
		for _, l := range links {
			fmt.Fprintf(&sb, " | %s dn=%.1f up=%.1f conns=%d", l.Label, l.Down*8/1e6, l.Up*8/1e6, l.Conns)
			if h, ok := health[l.Name]; ok {
				fmt.Fprintf(&sb, " probe=%v/%dms/fails%d", h.ok, h.delay, h.fails)
			}
		}
		sb.WriteString(dispatchInfo())
		if len(appList) > 0 {
			t := appList[0]
			fmt.Fprintf(&sb, " | top %s dn=%.1f links=", t.Process, t.Down*8/1e6)
			for n, c := range t.Links {
				fmt.Fprintf(&sb, "%s:%d ", labelFor(s, n), c)
			}
		}
		appendRates(sb.String())
		if len(dbgRows) > 0 && totDown > 2.5e6 {
			sort.Slice(dbgRows, func(i, j int) bool { return dbgRows[i].down > dbgRows[j].down })
			if len(dbgRows) > 16 {
				dbgRows = dbgRows[:16]
			}
			var cb strings.Builder
			cb.WriteString(time.Now().Format("15:04:05") + " conns:")
			for _, r := range dbgRows {
				fmt.Fprintf(&cb, " %s %.1f %s;", r.link, r.down*8/1e6, r.dst)
			}
			appendRates(cb.String())
		}
	}

	up := int64(0)
	if running {
		up = int64(time.Since(started).Seconds())
	}
	snap = Snapshot{Running: running, Version: version, App: appVersion, Error: errMsg, Uptime: up, Links: links, Apps: appList,
		TotalDown: totDown, TotalUp: totUp, PeakDown: peak, Conns: len(mc.Connections),
		Total: append([]float64(nil), totalHist...)}
	_, snap.Smart = dispatchSlotsOf("")
	snap.SameISP = sameProviderLines(links)
	snap.Activity = recentActivity(8)
	for _, dv := range devs {
		snap.Devices = append(snap.Devices, *dv)
	}
	sort.Slice(snap.Devices, func(i, j int) bool { return snap.Devices[i].IP < snap.Devices[j].IP })
}

func getSnapshot() Snapshot {
	statsMu.Lock()
	defer statsMu.Unlock()
	return snap
}

func rememberApps(apps map[string]*AppStat) {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	seen := map[string]bool{}
	for _, a := range settings.SeenApps {
		seen[a] = true
	}
	changed := false
	for p := range apps {
		if p != "system" && strings.HasSuffix(p, ".exe") && !seen[p] {
			settings.SeenApps = append(settings.SeenApps, p)
			seen[p] = true
			changed = true
		}
	}
	if changed {
		sort.Strings(settings.SeenApps)
		if len(settings.SeenApps) > 400 {
			settings.SeenApps = settings.SeenApps[:400]
		}
		saveSettingsLocked()
	}
}

func labelFor(s Settings, name string) string {
	for _, l := range s.Links {
		if l.Name == name {
			return l.Label
		}
	}
	return name
}

// rates.log: one line per second while traffic flows, so slow links can be diagnosed afterwards.
func appendRates(line string) {
	p := filepath.Join(rootDir, "data", "rates.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 4<<20 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\n")
	f.Close()
}
