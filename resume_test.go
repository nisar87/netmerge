package main

import (
	"testing"
	"time"
)

// A pause of NetMerge itself (a slow sample, a busy PC) is not a wake from sleep: reloading the engine for it
// cut every connection and could hold the next sample up again, reloading over and over. Windows' own power
// notices, or a pause of minutes, still count.
func TestWokeFromSleepNeedsWindowsOrALongGap(t *testing.T) {
	reset := func(asleep bool, at time.Time) {
		runMu.Lock()
		runAsleep, runPowerAt = asleep, at
		runMu.Unlock()
	}
	t.Cleanup(func() { reset(false, time.Time{}) })
	now := time.Now()

	reset(false, time.Time{})
	if wokeFromSleep(now.Add(-5*time.Second), now) {
		t.Fatal("a short gap is not sleep")
	}
	if wokeFromSleep(now.Add(-45*time.Second), now) {
		t.Fatal("a 45 s pause without a power notice is not sleep")
	}
	if !wokeFromSleep(now.Add(-3*time.Minute), now) {
		t.Fatal("a pause of minutes is sleep even without a power notice")
	}
	if wokeFromSleep(time.Time{}, now) {
		t.Fatal("the first tick is not a wake")
	}

	reset(false, now.Add(-time.Hour)) // a notice from an earlier sleep does not count
	if wokeFromSleep(now.Add(-45*time.Second), now) {
		t.Fatal("an old power notice made a pause count as sleep")
	}

	reset(true, now.Add(-44*time.Second)) // asleep, the wake notice not delivered yet
	if !wokeFromSleep(now.Add(-45*time.Second), now) {
		t.Fatal("Windows said the PC went to sleep")
	}

	reset(false, now.Add(-2*time.Second)) // the wake notice came in during the gap
	if !wokeFromSleep(now.Add(-45*time.Second), now) {
		t.Fatal("Windows said the PC woke")
	}

	reset(false, now.Add(-50*time.Second)) // going to sleep just before the last sample ended
	if !wokeFromSleep(now.Add(-45*time.Second), now) {
		t.Fatal("a notice just before the last sample counts")
	}
}

// powerSleep records when Windows sent the notice.
func TestPowerSleepRecordsTheTime(t *testing.T) {
	withTempLog(t)
	t.Cleanup(func() {
		runMu.Lock()
		runAsleep, runPowerAt = false, time.Time{}
		runMu.Unlock()
	})
	before := time.Now()
	powerSleep(true)
	runMu.Lock()
	at := runPowerAt
	runMu.Unlock()
	if at.Before(before) {
		t.Fatalf("power notice time not recorded: %v", at)
	}
}
