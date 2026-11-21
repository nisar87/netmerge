package main

import (
	"time"
)

// When the PC sleeps, the clock jumps. On waking, the network often needs a few seconds to come back and the
// engine's virtual adapter may be stale, so we rescan the adapters, forget old health results and reload the
// engine once the network is back.

const resumeGap = 20 * time.Second

func resumeDetected(last, now time.Time) bool { return !last.IsZero() && now.Sub(last) > resumeGap }

// resumeSureGap: a pause this long is taken as sleep even when Windows sent no power notice.
const resumeSureGap = 2 * time.Minute

// wokeFromSleep tells a real wake from NetMerge itself being held up. last is when the previous sample ended.
// A pause of 20 s to 2 min counts as sleep only when Windows said the PC went to sleep or woke around it;
// otherwise reloading the engine would cut every connection for nothing (and could hold the next sample up
// again, reloading over and over).
func wokeFromSleep(last, now time.Time) bool {
	if !resumeDetected(last, now) {
		return false
	}
	if now.Sub(last) >= resumeSureGap {
		return true
	}
	runMu.Lock()
	asleep, at := runAsleep, runPowerAt
	runMu.Unlock()
	return asleep || (!at.IsZero() && at.After(last.Add(-resumeGap)))
}

func onResume() {
	note("info", "The PC woke up - checking the connections again")
	logf("resume from sleep detected")
	probeMu.Lock()
	probes = map[string]probeState{}
	probeMu.Unlock()
	qualMu.Lock()
	qual = map[string][]qSample{}
	qualMu.Unlock()
	go safely("resume", func() {
		for i := 0; i < 3; i++ { // the network may need a few seconds
			time.Sleep(5 * time.Second)
			getAdapters(true)
		}
		if running, _, _, _ := engine.Status(); running {
			logf("resume: reloading the engine")
			logErr("reload the engine after sleep", engine.Apply())
		}
	})
}
