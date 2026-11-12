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
	baseDelay      int     // the line's usual response time when it is not struggling (slowly forgotten)
	congested      bool    // carrying far below its learned speed while its response time has blown up
}

// congestedNow tells whether a line is struggling right now: it carries traffic, but far less than it has
// shown it can, and its response time has risen to several times its usual one. That is a line that is full at
// a lower speed than usual (a congested ISP link, a router dropping packets), not a quiet line with room to spare:
// its learned speed must not draw new connections to it. A healthy line at full speed is never affected (its spare
// room is already near zero), and neither is a line whose response time stays normal.
func congestedNow(rx, capRx, tx, capTx float64, delay, base int) bool {
	return base > 0 && delay > 3*base && delay > base+150 &&
		rx+tx > 62500 /* 0.5 Mbit/s */ && rx < 0.5*capRx && tx < 0.5*capTx
}

// trackDelay keeps the line's usual response time: it follows lower readings at once and higher ones only
// slowly, so a few seconds of a struggling line do not become its "usual".
func trackDelay(base, delay int) int {
	switch {
	case delay <= 0:
		return base
	case base == 0 || delay < base:
		return delay
	default:
		return base + (delay-base)/50 + 1
	}
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
	dispCall   string // proxy the CALL group points at
	verifyN    int
	callBetter int
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
func learn(ring *[3]float64, n int, cap *float64, seen *bool, busyAll bool) {
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
	// a link that has not been seen carrying much, while the others are busy, is simply slow: remember that
	// instead of leaving it at the optimistic starting value
	if !*seen && busyAll && n >= 3 && avg >= 50e3 && avg < 1e6 {
		*seen, *cap = true, avg*1.5
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
	running, _, _, _ := engine.Status()
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
	ls = applyBusyRole(ls)
	if verifyN++; verifyN%10 == 0 && dispSeen != "" {
		verifySlots()
	}

	// the engine was (re)loaded: its selectors start again from their first member, so assert every slot again
	seen := engine.LoadID()
	force := seen != dispSeen
	if force {
		dispCall = ""
	}

	var set []*dispLink
	statsMu.Lock()
	allLoad := 0.0
	for _, l := range ls {
		if tr := tracks[l.Name]; tr != nil {
			allLoad += tr.rx + tr.tx
		}
	}
	busyAll := allLoad > 1.5e6
	callActive := false
	if !s.NoCallProtect {
		for _, a := range snap.Apps {
			if isCallApp(a.Process) && a.Down+a.Up > 20e3 {
				callActive = true
			}
		}
	}
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
			learn(&d.ringRx, d.n, &d.capRx, &d.seenRx, busyAll)
			learn(&d.ringTx, d.n, &d.capTx, &d.seenTx, busyAll)
		}
		if d.capRx == 0 {
			d.capRx, d.capTx = neutralCap, neutralCap
		}
		dl := health[l.Name].delay
		d.congested = congestedNow(d.emaRx, d.capRx, d.emaTx, d.capTx, dl, d.baseDelay)
		if !d.congested {
			d.baseDelay = trackDelay(d.baseDelay, dl)
		}
		// no line carries more than its adapter can: this also repairs a remembered speed that a counter
		// restart once inflated
		if a, ok := adapterByName(l.Name); ok {
			if c := rateCeiling(a.LinkRate); d.capRx > c || d.capTx > c {
				if d.capRx > c {
					d.capRx = c
				}
				if d.capTx > c {
					d.capTx = c
				}
			}
		}
		set = append(set, d)
	}
	statsMu.Unlock()
	sort.Slice(set, func(i, j int) bool { return set[i].name < set[j].name })

	// A link that has never been seen carrying anything is not assumed to be as fast as the best one: it starts at a
	// quarter of it and moves up as soon as it proves otherwise (a new mobile link must not take half the traffic).
	var bestRx, bestTx float64
	for _, d := range set {
		if d.seenRx && d.capRx > bestRx {
			bestRx = d.capRx
		}
		if d.seenTx && d.capTx > bestTx {
			bestTx = d.capTx
		}
	}
	for _, d := range set {
		if !d.seenRx && bestRx > 0 && d.capRx > bestRx/4 {
			d.capRx = bestRx / 4
		}
		if !d.seenTx && bestTx > 0 && d.capTx > bestTx/4 {
			d.capTx = bestTx / 4
		}
	}
	pen := delayPenalty(set)
	callName := ""
	if !s.NoCallProtect {
		callName = chooseCallLink(set)
	}

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
		if d.congested && rx+tx >= 100e3 {
			// full at a lower speed than usual: no spare room, whatever it carried before
			w = 0.1 * (d.capRx + d.capTx) / 2
		}
		w *= pen[i]
		if callActive && d.proxy == callName && callName != "" {
			w *= 0.3 // keep the call's line free: new downloads go elsewhere while the call runs
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
	counts := allocSlots(weights, dispatchSlots, slowFlags(set))
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
	explainSplit(set, cur, want, force)
	cur = map[string]int{}
	for _, p := range dispSlot {
		cur[p]++
	}
	setInfo(buildInfo(set, cur))
	publishSlots(set, cur)
}

// allocSlots splits n slots between links in proportion to their weights (largest remainder); every link keeps at
// least one slot so it is still used, and the result always adds up to n.
func allocSlots(weights []float64, n int, slow []bool) []int {
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
		if c < 1 && !(i < len(slow) && slow[i]) {
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
		} else if counts[r.i] == 1 && r.i < len(slow) && slow[r.i] {
			counts[r.i]--
			left++
		}
	}
	return counts
}

var (
	pubMu    sync.Mutex
	pubSlots = map[string]int{}
	pubSlow  = map[string]bool{} // judged far too slow to help (not merely without slots for a moment)
)

// dispatchSlowOf: the dispatcher judged this line far too slow to help. A line can also have no slots for a few
// seconds after it comes back (it is not in the split yet, or the split is held steady for 2 s); that is not "slow".
func dispatchSlowOf(name string) bool {
	pubMu.Lock()
	defer pubMu.Unlock()
	return pubSlow[name]
}

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
	pubSlow = map[string]bool{}
	slow := slowFlags(set)
	for i, d := range set {
		pubSlots[d.name] = cur[d.proxy]
		pubSlow[d.name] = slow[i]
	}
}

func setInfo(v string) {
	if v == "" {
		pubMu.Lock()
		pubSlots = map[string]int{}
		pubSlow = map[string]bool{}
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
		if d.congested {
			sb.WriteString("[congested]")
		}
	}
	fmt.Fprintf(&sb, " mv=%d", dispSwitch)
	dispSwitch = 0
	return sb.String()
}

// learned capacities survive a restart (data\\capacity.json), so the first minutes after start are not blind.
var capSaved time.Time

// capacity4.json: the earlier files (capacity.json, capacity3.json) held speeds inflated by false counter spikes (an
// adapter unplugged and plugged in again); such readings are now thrown away at the source (see ratecap.go)
// capKey names a line for the speed memory: the name together with the hardware, so a different adapter that
// ends up with the same name (a cable moved to another adapter, a Wi-Fi stick swapped) starts fresh.
func capKey(name string) string {
	if a, ok := adapterByName(name); ok {
		return name + "|" + a.Desc
	}
	return name
}

func capFile() string { return filepath.Join(rootDir, "data", "capacity4.json") }

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
	if v, ok := m[capKey(d.name)]; ok && v[0] > 0 && v[1] > 0 {
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
			m[capKey(d.name)] = [2]float64{d.capRx, d.capTx}
		}
	}
	if len(m) == 0 {
		return
	}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(capFile(), b, 0o644)
}

var (
	busyMu sync.Mutex
	busyOn = map[string]bool{}
)

// isStandby: the line has a role that keeps it out for now (backup while the others work, busy-only while they have room).
func isStandby(l Link) bool {
	switch l.Role {
	case "backup":
		for _, a := range activeLinks(getSettings()) {
			if a.Name == l.Name {
				return a.Dead && !isDead(l.Name) && !isQuotaOut(l.Name)
			}
		}
	case "busy":
		busyMu.Lock()
		defer busyMu.Unlock()
		s := getSettings()
		return !s.ClassicBalance && s.Mode == "fast" && !busyOn[l.Name]
	}
	return false
}

// applyBusyRole leaves out lines whose role is "busy" until the other lines are close to full, and switches them
// off again once the others have room (with a gap between the two limits so it does not flap).
func applyBusyRole(ls []activeLink) []activeLink {
	var main []activeLink
	hasBusy := false
	for _, l := range ls {
		if l.Role == "busy" {
			hasBusy = true
		} else {
			main = append(main, l)
		}
	}
	if !hasBusy || len(main) == 0 {
		return ls
	}
	statsMu.Lock()
	var load, capSum float64
	for _, l := range main {
		d := dispLinks[l.Name]
		if d == nil {
			continue
		}
		if d.capRx == 0 {
			capSum += neutralCap
		}
		if d.emaTx > d.emaRx {
			load += d.emaTx
			capSum += d.capTx
		} else {
			load += d.emaRx
			capSum += d.capRx
		}
	}
	statsMu.Unlock()
	ratio := 0.0
	if capSum > 0 {
		ratio = load / capSum
	}
	var out []activeLink
	for _, l := range ls {
		if l.Role != "busy" {
			out = append(out, l)
			continue
		}
		busyMu.Lock()
		wasOn := busyOn[l.Name]
		if busyOn[l.Name] && ratio < 0.5 {
			busyOn[l.Name] = false
		} else if !busyOn[l.Name] && ratio > 0.75 {
			busyOn[l.Name] = true
		}
		on := busyOn[l.Name]
		changed := on != wasOn
		busyMu.Unlock()
		if changed {
			if on {
				note("info", "%s joins: the other lines are nearly full (%.0f%% used)", labelFor(getSettings(), l.Name), ratio*100)
			} else {
				note("info", "%s stands by again: the other lines have room (%.0f%% used)", labelFor(getSettings(), l.Name), ratio*100)
			}
		}
		if on {
			out = append(out, l)
		}
	}
	return out
}

var lastSplitNote time.Time

// explainSplit writes one line to the activity list when the split between lines really changes, saying why
// (which line is busier). At most every 20 seconds, and never for the first assignment after a start.
func explainSplit(set []*dispLink, before, after map[string]int, force bool) {
	if force || len(set) < 2 || time.Since(lastSplitNote) < 20*time.Second {
		return
	}
	var up, down *dispLink
	bigUp, bigDown := 0, 0
	for _, d := range set {
		diff := after[d.proxy] - before[d.proxy]
		if diff >= 2 && diff > bigUp {
			up, bigUp = d, diff
		}
		if diff <= -2 && -diff > bigDown {
			down, bigDown = d, -diff
		}
	}
	if up == nil || down == nil {
		return
	}
	load := func(d *dispLink) float64 {
		a, b := 0.0, 0.0
		if d.capRx > 0 {
			a = d.emaRx / d.capRx
		}
		if d.capTx > 0 {
			b = d.emaTx / d.capTx
		}
		if b > a {
			a = b
		}
		return a * 100
	}
	s := getSettings()
	lastSplitNote = time.Now()
	note("info", "More new connections go to %s (%d/%d): %s is busier (%.0f%% used) than %s (%.0f%%)",
		labelFor(s, up.name), after[up.proxy], dispatchSlots, labelFor(s, down.name), load(down), labelFor(s, up.name), load(up))
}

// delayPenalty scales a link's weight down when its response time is much worse than the best line's, and when its
// quality is poor: a slow, laggy link (a weak mobile signal) makes pages hang when it gets its "fair" share.
func delayPenalty(set []*dispLink) []float64 {
	pen := make([]float64, len(set))
	delays := make([]int, len(set))
	best := 0
	statsMu.Lock()
	for i, d := range set {
		delays[i] = health[d.name].delay
		if delays[i] > 0 && (best == 0 || delays[i] < best) {
			best = delays[i]
		}
	}
	statsMu.Unlock()
	for i, d := range set {
		p := 1.0
		if delays[i] > 0 && best > 0 && delays[i] > 2*best {
			p = float64(2*best) / float64(delays[i])
			if p < 0.2 {
				p = 0.2
			}
		}
		qualMu.Lock()
		q := qualityOf(qual[d.name])
		qualMu.Unlock()
		if q.Label == "Poor" {
			p *= 0.5
		}
		pen[i] = p
	}
	return pen
}

// slowFlags marks lines that carry less than a tenth of the best line (once both are known): they get no slot of
// their own, so pages are not slowed down by sending a few of their connections down a weak line. They stay in the
// merge as standby and are still used when they are the only line left.
func slowFlags(set []*dispLink) []bool {
	out := make([]bool, len(set))
	// Compared direction by direction: a line is "too slow to help" only when it is under a tenth of the best line both
	// ways. Adding download and upload together wrongly wrote off a phone that downloads fine but had never been seen
	// uploading (18 Mbps down, 0.6 up, against 93 + 99 on the wired line: 18.8 < 19.2).
	var bestRx, bestTx float64
	for _, d := range set {
		if d.seenRx && d.capRx > bestRx {
			bestRx = d.capRx
		}
		if d.seenTx && d.capTx > bestTx {
			bestTx = d.capTx
		}
	}
	for i, d := range set {
		if !d.seenRx || bestRx == 0 || d.capRx >= bestRx/10 {
			continue
		}
		if d.seenTx && bestTx > 0 && d.capTx >= bestTx/10 {
			continue
		}
		out[i] = true
	}
	return out
}

func isCallApp(proc string) bool {
	for _, p := range callApps {
		if strings.EqualFold(p, proc) {
			return true
		}
	}
	return false
}

// chooseCallLink points CALL at the steadiest line (low delay, no loss, not slow) and keeps it there; it only moves
// when that line is gone or another one is clearly better for a few seconds in a row, so a running call is not
// moved around. Returns the proxy name now used.
func chooseCallLink(set []*dispLink) string {
	slow := slowFlags(set)
	score := func(i int) float64 {
		d := set[i]
		ms := float64(health[d.name].delay)
		if ms == 0 {
			ms = 200
		}
		qualMu.Lock()
		q := qualityOf(qual[d.name])
		qualMu.Unlock()
		v := ms + q.Loss*20 + float64(q.Jitter)
		if slow[i] {
			v += 1000
		}
		return v
	}
	best, cur := -1, -1
	statsMu.Lock()
	for i, d := range set {
		if best < 0 || score(i) < score(best) {
			best = i
		}
		if d.proxy == dispCall {
			cur = i
		}
	}
	statsMu.Unlock()
	if best < 0 {
		return ""
	}
	target := set[best].proxy
	if cur >= 0 && target != dispCall {
		if statsScore := func() bool { statsMu.Lock(); defer statsMu.Unlock(); return score(best) < 0.5*score(cur) }(); statsScore {
			callBetter++
		} else {
			callBetter = 0
		}
		if callBetter < 4 {
			target = dispCall
		}
	}
	if target != dispCall {
		if _, code, err := apiReq("PUT", "/proxies/CALL", map[string]string{"name": target}); err == nil && (code == 204 || code == 200) {
			dispCall, callBetter = target, 0
		} else {
			return dispCall
		}
	}
	return dispCall
}

// verifySlots asks the engine what each slot really points at. If it differs from what NetMerge set (the engine
// was reloaded, a call failed silently), the whole split is asserted again on the next tick.
func verifySlots() {
	b, code, err := apiReq("GET", "/proxies", nil)
	if err != nil || code != 200 {
		return
	}
	var r struct {
		Proxies map[string]struct {
			Now string `json:"now"`
		} `json:"proxies"`
	}
	if json.Unmarshal(b, &r) != nil {
		return
	}
	for i := 0; i < dispatchSlots; i++ {
		n := fmt.Sprintf("SLOT%d", i+1)
		if p, ok := r.Proxies[n]; ok && dispSlot[i] != "" && p.Now != dispSlot[i] {
			logf("slot check: %s points at %s in the engine but NetMerge set %s - setting it again", n, p.Now, dispSlot[i])
			dispSeen = ""
			return
		}
	}
}
