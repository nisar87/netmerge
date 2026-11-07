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

// Smart dispatcher. A connection can only ever use one link, so what matters is WHICH link each new connection
// gets - and a browser opens a whole burst of connections within a few milliseconds, so the choice has to be
// spread inside the burst, not decided once for all of them.
//
// The engine group AUTO deals new connections out, one after the other, over dispatchSlots small selectors
// (SLOT1..SLOT8). NetMerge decides how many of those slots point at each link and moves them with a tiny local
// API call, only when the split really changes:
//
//	free = capacity - current load, in the direction that is busy (never below 10% of capacity: a busy link is
//	       still tried now and then, so its capacity stays known)
//
// Capacity is learned from what each link has really carried (best 30 s average, forgotten slowly), separately
// for download and upload. No link is assumed faster than another: plan, ISP shaping and Wi-Fi quality show up
// in the numbers. Nothing is measured by extra traffic and nothing runs per connection.

const dispatchSlots = 8

type dispLink struct {
	name, proxy    string
	capRx, capTx   float64
	seenRx, seenTx bool
	emaRx, emaTx   float64
	ringRx, ringTx [3]float64
	n              int
	share          float64 // smoothed share of new connections
}

var (
	infoMu  sync.Mutex
	infoStr string

	// touched only by the dispatcher goroutine
	dispLinks  = map[string]*dispLink{}
	dispSlot   [dispatchSlots]string // proxy each slot points at (what the engine has)
	dispSeen   string
	dispLastAt time.Time
	dispWant   map[string]int
	dispWantN  int
	dispSwitch int
	dispShare  = map[string]float64{}
)

func dispatchLoop() {
	time.Sleep(3 * time.Second)
	t := time.NewTicker(time.Second)
	for range t.C {
		safely("dispatch", dispatchTick)
	}
}

// neutralCap is what a link is assumed to carry until it has really been seen carrying something: the same for
// every link, so nobody is favoured before there is data.
const neutralCap = 12.5e6 // 100 Mbit/s

// learn keeps the best 3-second average a link has carried (forgotten with a half-life of ~15 minutes). A short
// window follows the real speed up quickly and does not count idle seconds against the link; guessing too high
// only means the link is used a bit more, guessing too low would starve it.
func learn(ring *[3]float64, n int, cap *float64, seen *bool) {
	k := n
	if k > len(ring) {
		k = len(ring)
	}
	sum := 0.0
	for i := 0; i < k; i++ {
		sum += ring[i]
	}
	avg := sum / float64(k)
	if *cap == 0 {
		*cap = neutralCap
	}
	if avg >= 1e6 {
		if !*seen {
			*seen, *cap = true, avg
		}
		*cap *= 0.9992
		if avg > *cap {
			*cap = avg
		}
		if *cap < 1e6 {
			*cap = 1e6
		}
	}
}

func dispatchTick() {
	running, _, started, _ := engine.Status()
	s := getSettings()
	if !running || s.ClassicBalance || s.Mode == "stable" || strings.HasPrefix(s.Mode, "link:") {
		dispSeen = ""
		setInfo("")
		return
	}
	var ls []activeLink
	for _, l := range activeLinks(s) {
		if !l.Dead {
			ls = append(ls, l)
		}
	}
	if len(ls) < 2 {
		dispSeen = ""
		return
	}

	// the engine was (re)loaded: its selectors start again from their first member, so assert every slot again
	seen := fmt.Sprintf("%s|%d", engine.AppliedKey(), started.Unix())
	force := seen != dispSeen

	var set []*dispLink
	statsMu.Lock()
	for _, l := range ls {
		d := dispLinks[l.Name]
		if d == nil {
			d = &dispLink{name: l.Name}
			dispLinks[l.Name] = d
		}
		d.proxy = l.Proxy
		loadCaps(d)
		if tr := tracks[l.Name]; tr != nil {
			d.emaRx = 0.5*d.emaRx + 0.5*tr.rx
			d.emaTx = 0.5*d.emaTx + 0.5*tr.tx
			d.ringRx[d.n%3] = tr.rx
			d.ringTx[d.n%3] = tr.tx
			d.n++
			learn(&d.ringRx, d.n, &d.capRx, &d.seenRx)
			learn(&d.ringTx, d.n, &d.capTx, &d.seenTx)
		}
		if d.capRx == 0 {
			d.capRx, d.capTx = neutralCap, neutralCap
		}
		set = append(set, d)
	}
	statsMu.Unlock()
	sort.Slice(set, func(i, j int) bool { return set[i].name < set[j].name })

	// which direction is busy decides which spare capacity counts
	saveCaps(set)
	var rx, tx float64
	for _, d := range set {
		rx += d.emaRx
		tx += d.emaTx
	}
	weights := make([]float64, len(set))
	sum := 0.0
	for i, d := range set {
		var w float64
		switch {
		case rx+tx < 100e3: // idle: share by capacity
			w = d.capRx + d.capTx
		case tx > rx:
			w = d.capTx - d.emaTx
			if w < 0.1*d.capTx {
				w = 0.1 * d.capTx
			}
		default:
			w = d.capRx - d.emaRx
			if w < 0.1*d.capRx {
				w = 0.1 * d.capRx
			}
		}
		weights[i] = w
		sum += w
	}
	for i, d := range set {
		x := weights[i] / sum
		if d.share == 0 {
			d.share = x
		} else {
			d.share = 0.6*d.share + 0.4*x
		}
		weights[i] = d.share
	}
	sum = 0
	for _, w := range weights {
		sum += w
	}
	counts := allocSlots(weights, dispatchSlots)
	for i, d := range set {
		dispShare[d.name] = weights[i] / sum
	}
	// hysteresis: a new split must hold for 2 seconds (or be forced) before anything is moved
	want := map[string]int{}
	for i, d := range set {
		want[d.proxy] = counts[i]
	}
	cur := map[string]int{}
	for _, p := range dispSlot {
		cur[p]++
	}
	diff := 0
	for p, c := range want {
		if c > cur[p] {
			diff += c - cur[p]
		} else {
			diff += cur[p] - c
		}
	}
	if !force {
		if diff < 2 {
			dispWantN = 0
			setInfo(buildInfo(set, cur))
			publishSlots(set, cur)
			return
		}
		same := dispWant != nil
		for p, c := range want {
			if dispWant[p] != c {
				same = false
			}
		}
		dispWant = want
		if same {
			dispWantN++
		} else {
			dispWantN = 1
		}
		if dispWantN < 2 {
			setInfo(buildInfo(set, cur))
			publishSlots(set, cur)
			return
		}
	}
	dispWantN = 0

	// spread the slots evenly (smooth weighted round-robin), so even a burst of 3-4 connections is split
	seq := make([]string, 0, dispatchSlots)
	credit := make([]float64, len(set))
	for k := 0; k < dispatchSlots; k++ {
		best := 0
		for i := range set {
			credit[i] += float64(counts[i])
			if credit[i] > credit[best] {
				best = i
			}
		}
		credit[best] -= dispatchSlots
		seq = append(seq, set[best].proxy)
	}
	ok := true
	for i := 0; i < dispatchSlots; i++ {
		if !force && dispSlot[i] == seq[i] {
			continue
		}
		_, code, err := apiReq("PUT", fmt.Sprintf("/proxies/SLOT%d", i+1), map[string]string{"name": seq[i]})
		if err != nil || (code != 204 && code != 200) {
			ok = false
			continue
		}
		dispSlot[i] = seq[i]
		dispSwitch++
	}
	if ok {
		dispSeen = seen
	} else {
		dispSeen = ""
	}
	cur = map[string]int{}
	for _, p := range dispSlot {
		cur[p]++
	}
	setInfo(buildInfo(set, cur))
	publishSlots(set, cur)
}

// allocSlots splits n slots between links in proportion to their weights (largest remainder); every link keeps at
// least one slot so it is still used, and the result always adds up to n.
func allocSlots(weights []float64, n int) []int {
	counts := make([]int, len(weights))
	if len(weights) == 0 {
		return counts
	}
	sum := 0.0
	for _, w := range weights {
		if w > 0 {
			sum += w
		}
	}
	type rem struct {
		i int
		f float64
	}
	var rems []rem
	left := n
	for i, w := range weights {
		x := 0.0
		if sum > 0 && w > 0 {
			x = w / sum * float64(n)
		}
		c := int(x)
		if c < 1 {
			c = 1
		}
		counts[i] = c
		left -= c
		rems = append(rems, rem{i, x - float64(int(x))})
	}
	sort.Slice(rems, func(a, b int) bool { return rems[a].f > rems[b].f })
	for k := 0; left != 0 && k < 1000; k++ {
		r := rems[k%len(rems)]
		if left > 0 {
			counts[r.i]++
			left--
		} else if counts[r.i] > 1 {
			counts[r.i]--
			left++
		}
	}
	return counts
}

var (
	pubMu    sync.Mutex
	pubSlots = map[string]int{}
)

// dispatchSlotsOf: slots (of dispatchSlots) the smart dispatcher currently gives a connection; false when it is off.
func dispatchSlotsOf(name string) (int, bool) {
	pubMu.Lock()
	defer pubMu.Unlock()
	if len(pubSlots) == 0 {
		return 0, false
	}
	return pubSlots[name], true
}

func publishSlots(set []*dispLink, cur map[string]int) {
	pubMu.Lock()
	defer pubMu.Unlock()
	pubSlots = map[string]int{}
	for _, d := range set {
		pubSlots[d.name] = cur[d.proxy]
	}
}

func setInfo(v string) {
	if v == "" {
		pubMu.Lock()
		pubSlots = map[string]int{}
		pubMu.Unlock()
	}
	infoMu.Lock()
	infoStr = v
	infoMu.Unlock()
}

// dispatchInfo is appended to each line of rates.log: slots per link, learned capacity (down/up Mbps), moves.
func dispatchInfo() string {
	infoMu.Lock()
	defer infoMu.Unlock()
	return infoStr
}

func buildInfo(set []*dispLink, cur map[string]int) string {
	var sb strings.Builder
	sb.WriteString(" | auto")
	for _, d := range set {
		fmt.Fprintf(&sb, " %s=%d/%d(cap %.0f/%.0f)", d.name, cur[d.proxy], dispatchSlots, d.capRx*8/1e6, d.capTx*8/1e6)
	}
	fmt.Fprintf(&sb, " mv=%d", dispSwitch)
	dispSwitch = 0
	return sb.String()
}

// learned capacities survive a restart (data\\capacity.json), so the first minutes after start are not blind.
var capSaved time.Time

func capFile() string { return filepath.Join(rootDir, "data", "capacity.json") }

func loadCaps(d *dispLink) {
	if d.capRx != 0 {
		return
	}
	b, err := os.ReadFile(capFile())
	if err != nil {
		return
	}
	var m map[string][2]float64
	if json.Unmarshal(b, &m) != nil {
		return
	}
	if v, ok := m[d.name]; ok && v[0] > 0 && v[1] > 0 {
		d.capRx, d.capTx, d.seenRx, d.seenTx = v[0], v[1], true, true
	}
}

func saveCaps(set []*dispLink) {
	if time.Since(capSaved) < time.Minute {
		return
	}
	capSaved = time.Now()
	m := map[string][2]float64{}
	for _, d := range set {
		if d.seenRx && d.seenTx {
			m[d.name] = [2]float64{d.capRx, d.capTx}
		}
	}
	if len(m) == 0 {
		return
	}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(capFile(), b, 0o644)
}
