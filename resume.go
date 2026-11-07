package main

import (
	"time"
)

// When the PC sleeps, the clock jumps. On waking, the network often needs a few seconds to come back and the
// engine's virtual adapter may be stale, so we rescan the adapters, forget old health results and reload the
// engine once the network is back.

const resumeGap = 20 * time.Second

func resumeDetected(last, now time.Time) bool { return !last.IsZero() && now.Sub(last) > resumeGap }

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
			_ = engine.Apply()
		}
	})
}
