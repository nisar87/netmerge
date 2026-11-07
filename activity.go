package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	actMu    sync.Mutex
	actLog   []ActivityEvent
	actDirty bool
)

const actKeep = 500 // events kept (and saved) for the History page

func eventsPath() string { return filepath.Join(rootDir, "data", "events.json") }

// eventsLoop loads the saved history once and saves it again when something new was added.
func eventsLoop() {
	if b, err := os.ReadFile(eventsPath()); err == nil {
		var old []ActivityEvent
		if json.Unmarshal(b, &old) == nil {
			actMu.Lock()
			actLog = append(old, actLog...)
			if len(actLog) > actKeep {
				actLog = actLog[len(actLog)-actKeep:]
			}
			actMu.Unlock()
		}
	}
	t := time.NewTicker(20 * time.Second)
	for range t.C {
		eventsSave()
	}
}

func eventsSave() {
	actMu.Lock()
	if !actDirty {
		actMu.Unlock()
		return
	}
	b, _ := json.Marshal(actLog)
	actDirty = false
	actMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(eventsPath()), 0o755)
	tmp := eventsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, eventsPath())
	}
}

func allEvents() []ActivityEvent {
	actMu.Lock()
	defer actMu.Unlock()
	out := make([]ActivityEvent, 0, len(actLog))
	for i := len(actLog) - 1; i >= 0; i-- {
		out = append(out, actLog[i])
	}
	return out
}

func note(kind, format string, a ...any) {
	e := ActivityEvent{Time: time.Now().Unix(), Kind: kind, Text: fmt.Sprintf(format, a...)}
	actMu.Lock()
	actLog = append(actLog, e)
	if len(actLog) > actKeep {
		actLog = actLog[len(actLog)-actKeep:]
	}
	actDirty = true
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
