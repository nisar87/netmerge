package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// A slow disk must not hold up the sampler: keepRate hands rates.log lines to a writer of their own, and drops
// lines (counting them) when that writer is far behind instead of waiting.
func TestKeepRateDoesNotWaitForTheDisk(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	var mu sync.Mutex
	var written []string
	old := rateWrite
	rateWrite = func(s string) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release // the disk is stuck
		mu.Lock()
		written = append(written, strings.Split(s, "\n")...)
		mu.Unlock()
	}
	defer func() { rateWrite = old }()

	// the writer takes a first line and gets stuck on the disk; only then does the flood come
	keepRate("first", true)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the rates.log writer never started")
	}
	start := time.Now()
	for i := 0; i < 2000; i++ {
		keepRate("rate line", true)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("keepRate waited for the disk: %v for 2000 lines", d)
	}
	if got := len(strings.Split(string(recentRatesText()), "\n")); got != 600 {
		t.Fatalf("recent rate lines kept = %d, want 600", got)
	}
	if rateDropped.Load() == 0 {
		t.Fatal("lines beyond the writer's queue were not counted as dropped")
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(written)
		mu.Unlock()
		if n >= 512 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d lines reached rates.log", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	keepRate("after", true)
	deadline = time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		last := ""
		if len(written) > 0 {
			last = written[len(written)-1]
		}
		mu.Unlock()
		if last == "after" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a line written after the disk recovered never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rateDropped.Load() != 0 {
		t.Fatal("the dropped count was not reported once the writer caught up")
	}
}
