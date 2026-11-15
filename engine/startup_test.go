package engine

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// A start that hangs names the step it hangs on, so NetMerge's "The engine did not respond" quotes it.
func TestSlowStartNamesTheStep(t *testing.T) {
	old := slowStartAfter
	slowStartAfter = 50 * time.Millisecond
	defer func() { slowStartAfter = old }()
	var out syncBuf
	e := New(t.TempDir(), &out)
	finished := e.watchStart()
	e.startStep("reading the settings")
	e.startStep("creating the network adapter")
	time.Sleep(slowStepOver + 100*time.Millisecond)
	e.startStep("adding the routes")
	if !strings.Contains(out.String(), "start-up is slow") || !strings.Contains(out.String(), "on: creating the network adapter") {
		t.Fatalf("no slow-start line naming the step:\n%s", out.String())
	}
	finished()
	if !strings.Contains(out.String(), "start-up took") || !strings.Contains(out.String(), "slow steps: [creating the network adapter 2.") {
		t.Fatalf("no summary naming the slow step:\n%s", out.String())
	}
}

// A quick start writes nothing, and steps after the start (reloads) are not tracked.
func TestQuickStartIsQuiet(t *testing.T) {
	var out syncBuf
	e := New(t.TempDir(), &out)
	finished := e.watchStart()
	e.startStep("reading the settings")
	finished()
	e.ready.Store(true)
	e.startStep("reading the settings")
	if name, _ := e.currentStep(); name != "" {
		t.Fatalf("a reload changed the start-up step to %q", name)
	}
	if s := out.String(); s != "" {
		t.Fatalf("a quick start wrote: %s", s)
	}
}
