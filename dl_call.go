package main

import (
	"sync"
	"time"
)

// What the dispatcher knows about a running call, shared with the download manager: downloads keep off the call's line.
var (
	callMu   sync.Mutex
	callSeen time.Time
	callLn   string
)

func publishCall(active bool, line string) {
	if !active {
		return
	}
	callMu.Lock()
	callSeen, callLn = time.Now(), line
	callMu.Unlock()
}

// dlCallNow reports whether a call is running (it stays "running" for 20 s after the last sign of it, so a quiet
// moment in a meeting does not make downloads jump back onto the line) and which line it uses.
func dlCallNow() (bool, string) {
	callMu.Lock()
	defer callMu.Unlock()
	return time.Since(callSeen) < 20*time.Second, callLn
}
