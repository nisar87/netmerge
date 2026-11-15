package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempLog points the data folder and netmerge.log at a temporary folder and returns a reader for the log.
func withTempLog(t *testing.T) func() string {
	t.Helper()
	oldRoot, oldLog := rootDir, logFile
	rootDir = t.TempDir()
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	f, err := os.Create(dataPath("netmerge.log"))
	if err != nil {
		t.Fatal(err)
	}
	logMu.Lock()
	logFile = f
	logMu.Unlock()
	t.Cleanup(func() {
		logMu.Lock()
		logFile = oldLog
		logMu.Unlock()
		f.Close()
		rootDir = oldRoot
		runMu.Lock()
		runAsleep, runEnding = false, ""
		runMu.Unlock()
	})
	return func() string { b, _ := os.ReadFile(dataPath("netmerge.log")); return string(b) }
}

func writeMarker(t *testing.T, m runMarker) {
	b, _ := json.Marshal(m)
	if err := os.WriteFile(dataPath("running.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A run that Windows ended (shut down with Fast Startup keeps the boot time, so the boot check alone misses it) is
// not reported as a crash.
func TestLastRunEndedByWindowsIsNotACrash(t *testing.T) {
	read := withTempLog(t)
	now := time.Now()
	writeMarker(t, runMarker{Version: "3.21.3", Started: now.Add(-5 * time.Hour).Unix(), Alive: now.Add(-10 * time.Minute).Unix(), Ending: sessionEndReason(0)})
	checkLastRun()
	log := read()
	if !strings.Contains(log, "Windows shut down or restarted") || strings.Contains(log, "unexpectedly") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestLastRunEndedWhileAsleep(t *testing.T) {
	read := withTempLog(t)
	now := time.Now()
	writeMarker(t, runMarker{Version: "3.21.3", Started: now.Add(-5 * time.Hour).Unix(), Alive: now.Add(-10 * time.Minute).Unix(), Asleep: true})
	checkLastRun()
	if log := read(); !strings.Contains(log, "ended while the PC was asleep") || strings.Contains(log, "unexpectedly") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestLastRunWithoutWordFromWindowsIsUnexpected(t *testing.T) {
	read := withTempLog(t)
	now := time.Now()
	writeMarker(t, runMarker{Version: "3.21.3", Started: now.Add(-5 * time.Minute).Unix(), Alive: now.Add(-time.Minute).Unix()})
	checkLastRun()
	if log := read(); !strings.Contains(log, "ended unexpectedly") {
		t.Fatalf("log:\n%s", log)
	}
}

// Sleep and wake are written once each and kept in the run marker while asleep.
func TestPowerSleepMarksTheRun(t *testing.T) {
	read := withTempLog(t)
	powerSleep(true)
	powerSleep(true)
	var m runMarker
	b, _ := os.ReadFile(dataPath("running.json"))
	_ = json.Unmarshal(b, &m)
	if !m.Asleep {
		t.Fatalf("marker not asleep: %s", b)
	}
	powerSleep(false)
	b, _ = os.ReadFile(dataPath("running.json"))
	m = runMarker{}
	_ = json.Unmarshal(b, &m)
	if m.Asleep {
		t.Fatalf("marker still asleep after waking: %s", b)
	}
	log := read()
	if strings.Count(log, "going to sleep") != 1 || strings.Count(log, "woke from sleep") != 1 {
		t.Fatalf("log:\n%s", log)
	}
}

func TestSessionEndReason(t *testing.T) {
	for flags, want := range map[uintptr]string{0: "shut down or restarted", 0x80000000: "signed out", 0x1: "installer or update"} {
		if got := sessionEndReason(flags); !strings.Contains(got, want) {
			t.Errorf("flags %#x: %q", flags, got)
		}
	}
}
