package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// NetMerge only ever closes engine processes it started itself: the one recorded in data/engine.pid
// (checked against its program path, so a reused process number is never touched) and a mihomo that runs
// from NetMerge's own core folder. Another program's mihomo is left alone.

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

// recordedIsOwn tells whether the recorded engine is NetMerge's own (this program run with -engine).
func recordedIsOwn() bool {
	_, exe, ok := recordedEngine()
	self, _ := os.Executable()
	return ok && samePath(exe, self)
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

// quitOwnEngine asks NetMerge's own engine to stop by itself, so it puts back the PC's routes and DNS.
func quitOwnEngine(pid int) bool {
	if _, code, err := apiReq("POST", "/quit", nil); err != nil || code >= 300 {
		return false
	}
	return waitGone(pid, 5*time.Second)
}

// stopOwnEngines closes the engines NetMerge started: the recorded one (cleanly when it is NetMerge's own)
// and any mihomo running from NetMerge's core folder.
func stopOwnEngines() {
	if pid, exe, ok := recordedEngine(); ok {
		self, _ := os.Executable()
		if !samePath(exe, self) || !quitOwnEngine(pid) {
			killPid(pid)
			waitGone(pid, 2*time.Second)
		}
	}
	core := mihomoExe()
	for _, pid := range processesRunning(core) {
		killPid(pid)
	}
	forgetEngine()
}

// stopEngineProcess stops an engine NetMerge started in this session.
func stopEngineProcess(c interface{ Pid() int }, own bool) {
	pid := c.Pid()
	if own && quitOwnEngine(pid) {
		return
	}
	killPid(pid)
	waitGone(pid, 2*time.Second)
}
