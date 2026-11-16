package main

import (
	"testing"
	"time"
)

func TestStepClockNamesTheSlowSteps(t *testing.T) {
	var c stepClock
	t0 := time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)
	c.reset(t0)
	c.step("working out the numbers for the window", t0.Add(100*time.Millisecond))
	c.step("waiting for the next tick", t0.Add(1*time.Second))
	c.step("reading the adapter list", t0.Add(3100*time.Millisecond))
	c.step("reading the byte counters", t0.Add(3200*time.Millisecond))
	c.step("waiting for the next tick", t0.Add(3500*time.Millisecond)) // the same step twice adds up
	got := c.slowest(300 * time.Millisecond)
	want := "[reading the adapter list 2.1s, waiting for the next tick 1.2s]"
	if got != want {
		t.Fatalf("slowest = %q, want %q", got, want)
	}

	c.reset(t0.Add(4 * time.Second))
	if got := c.slowest(300 * time.Millisecond); got != "[no step took long]" {
		t.Fatalf("after reset: %q", got)
	}
	// a clock that went backwards (or no mark yet) adds nothing
	var fresh stepClock
	fresh.step("starting a sample", t0)
	fresh.step("reading the adapter list", t0.Add(-time.Second))
	if len(fresh.spent) != 0 {
		t.Fatalf("time counted without a mark or backwards: %v", fresh.spent)
	}
}
