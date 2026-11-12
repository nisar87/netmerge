//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Run by CI on a Windows machine: finding, naming and closing processes by their program path, and the
// 64-bit adapter counters.
func TestProcessesByPath(t *testing.T) {
	old := rootDir
	rootDir = t.TempDir()
	defer func() { rootDir = old }()
	os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	self, _ := os.Executable()
	if !samePath(processPath(os.Getpid()), self) {
		t.Fatalf("own path %q, want %q", processPath(os.Getpid()), self)
	}
	ping, err := exec.LookPath("ping")
	if err != nil {
		t.Skip("no ping")
	}
	c := exec.Command(ping, "-n", "60", "127.0.0.1")
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	go c.Wait()
	found := false
	for _, p := range processesRunning(processPath(c.Process.Pid)) {
		found = found || p == c.Process.Pid
	}
	if !found {
		t.Fatal("the child was not found by its path")
	}
	rememberEngine(c.Process.Pid, processPath(c.Process.Pid))
	if _, _, ok := recordedEngine(); !ok {
		t.Fatal("recorded process not recognised")
	}
	killPid(c.Process.Pid)
	if !waitGone(c.Process.Pid, 3*time.Second) {
		t.Fatal("process still running after kill")
	}
	if _, _, ok := recordedEngine(); ok {
		t.Fatal("a finished process is still recognised")
	}
	forgetEngine()
}

func TestCounters64(t *testing.T) {
	c64, c32 := ifCounters64(), ifCounters()
	if len(c64) == 0 {
		t.Fatal("no adapters")
	}
	matched := 0
	for idx, v := range c32 {
		w, ok := c64[idx]
		if !ok || v[0] == 0 {
			continue
		}
		// the same counter, read moments apart: the low 32 bits agree closely
		if d := uint32(w[0]) - v[0]; d < 50<<20 {
			matched++
		}
	}
	if matched == 0 {
		t.Fatalf("64-bit counters do not match the 32-bit ones: %v vs %v", c64, c32)
	}
	if b := bootTime(); b.IsZero() || b.After(time.Now()) {
		t.Fatalf("boot time %v", b)
	}
}
