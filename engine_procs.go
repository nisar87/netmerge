package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// NetMerge only ever closes engine processes it started itself: the one recorded in data/engine.pid (checked
// against its program path, so a reused process number is never touched), and a program still running from the
// "core" folder where versions before 3.18 kept a separately downloaded engine.

func enginePidFile() string { return filepath.Join(rootDir, "data", "engine.pid") }

func rememberEngine(pid int, exe string) {
	_ = os.WriteFile(enginePidFile(), []byte(strconv.Itoa(pid)+"\n"+exe+"\n"), 0o644)
}

func forgetEngine() { _ = os.Remove(enginePidFile()) }

// recordedEngine returns the engine recorded in the pid file if that process is still running the same program.
func recordedEngine() (pid int, exe string, ok bool) {
	b, err := os.ReadFile(enginePidFile())
	if err != nil {
		return 0, "", false
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 2 {
		return 0, "", false
	}
	pid, _ = strconv.Atoi(strings.TrimSpace(lines[0]))
	exe = strings.TrimSpace(lines[1])
	if pid <= 0 || pid == os.Getpid() || !samePath(processPath(pid), exe) {
		return 0, "", false
	}
	return pid, exe, true
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// waitGone waits up to d for the process to exit.
func waitGone(pid int, d time.Duration) bool {
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		if processPath(pid) == "" {
			return true
		}
	}
	return processPath(pid) == ""
}

// quitOwnEngine asks the engine to stop by itself, so it puts back the PC's routes and DNS.
func quitOwnEngine(pid int) bool {
	if _, code, err := apiReq("POST", "/quit", nil); err != nil || code >= 300 {
		return false
	}
	return waitGone(pid, 5*time.Second)
}

// stopOwnEngines closes the engines NetMerge started: the recorded one (cleanly when it is the built-in engine)
// and any program running from the old core folder.
func stopOwnEngines() {
	if pid, exe, ok := recordedEngine(); ok {
		self, _ := os.Executable()
		if !samePath(exe, self) || !quitOwnEngine(pid) {
			killPid(pid)
			waitGone(pid, 2*time.Second)
		}
	}
	for _, pid := range processesIn(oldCoreDir()) {
		killPid(pid)
	}
	forgetEngine()
}

// stopEngineProcess stops the engine NetMerge started in this session (cleanly first).
func stopEngineProcess(c interface{ Pid() int }) {
	pid := c.Pid()
	if quitOwnEngine(pid) {
		return
	}
	killPid(pid)
	waitGone(pid, 2*time.Second)
}

// removeOldEngine deletes the separately downloaded engine of versions before 3.18 (about 60 MB), once nothing
// runs from it any more.
func removeOldEngine() {
	dir := oldCoreDir()
	if _, err := os.Stat(dir); err != nil {
		return
	}
	for _, pid := range processesIn(dir) {
		killPid(pid)
		waitGone(pid, 2*time.Second)
	}
	if err := os.RemoveAll(dir); err == nil {
		logf("removed the engine program older versions downloaded (core folder)")
	}
}

// processesIn lists processes whose program is inside dir.
func processesIn(dir string) []int {
	var out []int
	for _, pid := range allPids() {
		if p := processPath(pid); p != "" && pathInside(p, dir) {
			out = append(out, pid)
		}
	}
	return out
}

func pathInside(p, dir string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	if runtime.GOOS == "windows" {
		rel, err = filepath.Rel(strings.ToLower(filepath.Clean(dir)), strings.ToLower(filepath.Clean(p)))
	}
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}
