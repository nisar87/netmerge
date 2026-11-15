package engine

import (
	"sync"
	"time"
)

// The start-up steps are named while they run, so a start that hangs says where: NetMerge gives the engine about
// 15 seconds to answer and then stops it, quoting the engine's last lines.

// slowStartAfter is when a start that has not finished first writes the step it is on (a variable for tests).
var slowStartAfter = 5 * time.Second

type startSteps struct {
	mu    sync.Mutex
	name  string
	since time.Time
	slow  []string // steps that took slowStepOver or longer, with their time
}

const slowStepOver = 2 * time.Second

// startStep marks the beginning of a start-up step (and the end of the one before).
func (e *Engine) startStep(name string) {
	if e.ready.Load() {
		return // a reload, not the start
	}
	e.nextStep(name)
}

func (e *Engine) nextStep(name string) {
	s := &e.steps
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.name != "" && now.Sub(s.since) >= slowStepOver {
		s.slow = append(s.slow, s.name+" "+now.Sub(s.since).Round(100*time.Millisecond).String())
	}
	s.name, s.since = name, now
}

func (e *Engine) currentStep() (string, time.Duration) {
	s := &e.steps
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.name, time.Since(s.since)
}

// watchStart writes, every few seconds while the start has not finished, which step it is on and for how long.
// It returns a function to call when the start is over.
func (e *Engine) watchStart() func() {
	done := make(chan struct{})
	began := time.Now()
	go func() {
		t := time.NewTicker(slowStartAfter)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				name, d := e.currentStep()
				e.log.Warnf("start-up is slow: %v so far, %v of it on: %s", time.Since(began).Round(100*time.Millisecond), d.Round(100*time.Millisecond), name)
			}
		}
	}()
	return func() {
		close(done)
		e.nextStep("")
		e.steps.mu.Lock()
		slow := e.steps.slow
		e.steps.mu.Unlock()
		if total := time.Since(began); total >= slowStartAfter || len(slow) > 0 {
			e.log.Warnf("start-up took %v; slow steps: %v", total.Round(100*time.Millisecond), slow)
		}
	}
}
