package main

import (
	"testing"
	"time"
)

func setRunEnding(v string) {
	runMu.Lock()
	runEnding = v
	runMu.Unlock()
}

// Windows closes the engine (exit code 0x40010004) a moment before it tells NetMerge the session is ending; that is
// not a crash.
func TestEngineClosedByWindowsAtShutdownIsNotACrash(t *testing.T) {
	t.Cleanup(func() { setRunEnding("") })
	setRunEnding("")
	go func() {
		time.Sleep(200 * time.Millisecond)
		setRunEnding(sessionEndReason(0))
	}()
	if !endedWithSession(0x40010004, 3*time.Second) {
		t.Fatal("engine closed by Windows at shutdown was treated as a crash")
	}
}

func TestEngineExitWhileSessionEndsIsNotACrash(t *testing.T) {
	t.Cleanup(func() { setRunEnding("") })
	setRunEnding(sessionEndReason(0))
	if !endedWithSession(2, 0) {
		t.Fatal("engine exit during the end of the session was treated as a crash")
	}
}

func TestEngineCrashIsStillACrash(t *testing.T) {
	setRunEnding("")
	start := time.Now()
	if endedWithSession(2, 3*time.Second) {
		t.Fatal("a real crash was not treated as one")
	}
	if time.Since(start) > time.Second {
		t.Fatal("a real crash waited for the end of the session")
	}
	// The session-end code without word from Windows (ended some other way) is still a crash.
	if endedWithSession(0x40010004, 200*time.Millisecond) {
		t.Fatal("0x40010004 without the session ending was not treated as a crash")
	}
}
