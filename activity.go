package main

import (
	"fmt"
	"sync"
	"time"
)

// A short list of what NetMerge decided and why ("Wi-Fi left out: no internet"), shown on the Overview page so
// nothing happens silently.

type ActivityEvent struct {
	Time int64  `json:"time"`
	Kind string `json:"kind"` // info | good | warn | bad
	Text string `json:"text"`
}

var (
	actMu  sync.Mutex
	actLog []ActivityEvent
)

func note(kind, format string, a ...any) {
	e := ActivityEvent{Time: time.Now().Unix(), Kind: kind, Text: fmt.Sprintf(format, a...)}
	actMu.Lock()
	actLog = append(actLog, e)
	if len(actLog) > 60 {
		actLog = actLog[len(actLog)-60:]
	}
	actMu.Unlock()
}

// recentActivity returns the newest n events, newest first.
func recentActivity(n int) []ActivityEvent {
	actMu.Lock()
	defer actMu.Unlock()
	var out []ActivityEvent
	for i := len(actLog) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, actLog[i])
	}
	return out
}
