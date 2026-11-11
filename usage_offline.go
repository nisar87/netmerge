package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Data used while NetMerge was closed (or before it started after a crash) is still counted: NetMerge remembers
// each adapter's 64-bit byte counters as of the last bytes it counted, and on the next start adds the difference,
// as long as Windows has not restarted in between (the counters start again from zero then) and the adapter is
// the same one. The bytes are added to the day NetMerge starts again.

type ctrLink struct {
	Idx  int    `json:"idx"`
	Desc string `json:"desc"`
	Rx   uint64 `json:"rx"`
	Tx   uint64 `json:"tx"`
}

type ctrSnap struct {
	Boot  int64              `json:"boot"` // unix seconds when Windows started
	At    int64              `json:"at"`
	Links map[string]ctrLink `json:"links"`
}

var (
	ctrNow     map[string]ctrLink // guarded by usageMu
	offlineRan bool               // touched only by the sampler
)

func countersPath() string { return filepath.Join(rootDir, "data", "counters.json") }

// usageSetCounters records the counters that match what has been added to the usage so far.
func usageSetCounters(m map[string]ctrLink) {
	usageMu.Lock()
	ctrNow = m
	usageMu.Unlock()
}

// saveCountersLocked writes the counters next to the usage (called with usageMu held, right after the usage).
func saveCountersLocked() {
	if ctrNow == nil {
		return
	}
	b, _ := json.Marshal(ctrSnap{Boot: bootTime().Unix(), At: time.Now().Unix(), Links: ctrNow})
	tmp := countersPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, countersPath())
	}
}

// maxLineRate is a speed no single connection reaches (10 Gbit/s): a difference above it means the counter is
// not the one that was saved, and nothing is added.
const maxLineRate = 1.25e9

// offlineUsage returns, per link, the bytes the adapter carried since the saved counters.
func offlineUsage(snap ctrSnap, boot, now time.Time, adapters map[string]Adapter, c64 map[int][2]uint64) map[string][2]uint64 {
	out := map[string][2]uint64{}
	if d := boot.Unix() - snap.Boot; d > 120 || d < -120 {
		return out // Windows restarted: the counters started again from zero
	}
	elapsed := now.Sub(time.Unix(snap.At, 0)).Seconds()
	if elapsed < 0 {
		return out
	}
	limit := uint64(maxLineRate * (elapsed + 60))
	for name, l := range snap.Links {
		a, ok := adapters[name]
		if !ok || a.Index != l.Idx || a.Desc != l.Desc {
			continue
		}
		c, ok := c64[a.Index]
		if !ok || c[0] < l.Rx || c[1] < l.Tx {
			continue // the adapter was reset (unplugged and plugged in again)
		}
		rx, tx := c[0]-l.Rx, c[1]-l.Tx
		if rx+tx == 0 || rx > limit || tx > limit {
			continue
		}
		out[name] = [2]uint64{rx, tx}
	}
	return out
}

// countOfflineUsage runs once, on the sampler's first reading.
func countOfflineUsage(adapters map[string]Adapter, c64 map[int][2]uint64) {
	if offlineRan {
		return
	}
	offlineRan = true
	b, err := os.ReadFile(countersPath())
	if err != nil {
		return
	}
	var snap ctrSnap
	if json.Unmarshal(b, &snap) != nil {
		return
	}
	for name, v := range offlineUsage(snap, bootTime(), time.Now(), adapters, c64) {
		logf("%s: counted %d bytes in, %d out used while NetMerge was not running", name, v[0], v[1])
		usageAdd(name, v[0], v[1])
	}
}
