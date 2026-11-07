package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	mu            sync.Mutex
	applyMu       sync.Mutex // serialises config reloads
	cmd           *exec.Cmd
	running       bool
	startedAt     time.Time
	lastError     string
	version       string
	restarts      int
	restartWindow time.Time
	appliedKey    string
	apiFails      int
}

var engine = &Engine{}

func mihomoExe() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(rootDir, "core", "mihomo.exe")
	}
	return filepath.Join(rootDir, "core", "mihomo")
}

var apiClient = &http.Client{Timeout: 4 * time.Second}

func apiReq(method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://"+apiAddr+path, rd)
	req.Header.Set("Authorization", "Bearer "+getSettings().Secret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := apiClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

func engineAlive() (string, bool) {
	b, code, err := apiReq("GET", "/version", nil)
	if err != nil || code != 200 {
		return "", false
	}
	var v struct {
		Version string `json:"version"`
	}
	_ = json.Unmarshal(b, &v)
	return v.Version, true
}

// activeKey identifies the set of connections (and shares) the engine should currently be using.
func activeKey() string {
	var parts []string
	for _, l := range activeLinks(getSettings()) {
		parts = append(parts, fmt.Sprintf("%s:%d:%v", l.Name, l.Weight, l.Dead))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (e *Engine) Status() (bool, string, time.Time, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running, e.lastError, e.startedAt, e.version
}

func (e *Engine) AppliedKey() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.appliedKey
}

// Adopt an engine left running by a previous NetMerge session.
func (e *Engine) Adopt() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := engineAlive(); ok {
		e.running, e.version, e.startedAt = true, v, time.Now()
		logf("re-attached to running engine %s", v)
		return
	}
	// a mihomo we cannot talk to (e.g. one left over from an old session) - remove it
	killProcessByName("mihomo.exe")
}

func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		return nil
	}
	if _, err := os.Stat(mihomoExe()); err != nil {
		e.lastError = "The network engine is not installed yet. NetMerge downloads it by itself - try again in a minute."
		if st, _, _ := engineDownloadState(); st == "" {
			go safely("engine download", downloadEngine)
		}
		return fmt.Errorf("%s", e.lastError)
	}
	if msg := detectConflictNow(); msg != "" {
		e.lastError = msg
		logf("not starting: %s", msg)
		return fmt.Errorf("%s", msg)
	}
	links := activeLinks(getSettings())
	if len(links) == 0 {
		e.lastError = "No internet connection found. Plug in or connect at least one, then try again."
		return fmt.Errorf("%s", e.lastError)
	}
	killProcessByName("mihomo.exe")
	if err := writeConfig(); err != nil {
		e.lastError = "Could not write config: " + err.Error()
		return err
	}
	syncShareFirewall()
	logPath := filepath.Join(rootDir, "data", "engine.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 8<<20 {
		_ = os.Rename(logPath, logPath+".old")
	}
	lf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	cmd := hiddenCmd(mihomoExe(), "-d", configDir())
	pr, pw, perr := os.Pipe()
	if perr != nil {
		cmd.Stdout, cmd.Stderr = lf, lf
	} else {
		cmd.Stdout, cmd.Stderr = pw, pw
	}
	if err := cmd.Start(); err != nil {
		if perr == nil {
			pr.Close()
			pw.Close()
		}
		if lf != nil {
			lf.Close()
		}
		e.lastError = "Could not start engine: " + err.Error()
		return err
	}
	if perr == nil {
		pw.Close() // the engine holds its own copy; we read until it exits
		go pumpEngineLog(pr, lf)
	}
	e.cmd = cmd
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		if perr != nil && lf != nil {
			lf.Close()
		}
	}()

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			e.lastError = "The engine stopped right after starting (" + fmt.Sprint(err) + "). " + tailFile(logPath, 6)
			e.cmd = nil
			logf("engine failed: %s", e.lastError)
			return fmt.Errorf("%s", e.lastError)
		default:
		}
		if v, ok := engineAlive(); ok {
			e.running, e.version, e.startedAt, e.lastError, e.apiFails = true, v, time.Now(), "", 0
			e.appliedKey = keyOf(links)
			logf("engine %s started with %d connection(s)", v, len(links))
			go e.watch(cmd, exited)
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	e.cmd = nil
	e.lastError = "The engine did not respond. " + tailFile(logPath, 6)
	return fmt.Errorf("%s", e.lastError)
}

func keyOf(links []activeLink) string {
	var parts []string
	for _, l := range links {
		parts = append(parts, fmt.Sprintf("%s:%d:%v", l.Name, l.Weight, l.Dead))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

// watch restarts the engine if it dies while it should be running. It only acts for the process
// it was started for, so a deliberate stop/start never makes an old watcher kill the new engine.
func (e *Engine) watch(cmd *exec.Cmd, exited chan error) {
	err := <-exited
	e.mu.Lock()
	current := e.cmd == cmd && e.running
	if current {
		e.running, e.cmd = false, nil
	}
	e.mu.Unlock()
	if current {
		e.restartAfter(fmt.Sprintf("engine exited unexpectedly (%v)", err))
	}
}

// restartAfter restarts a dead engine, but gives up (and restores normal internet) if it keeps dying.
func (e *Engine) restartAfter(why string) {
	e.mu.Lock()
	if time.Since(e.restartWindow) > 2*time.Minute {
		e.restartWindow, e.restarts = time.Now(), 0
	}
	e.restarts++
	tooMany := e.restarts > 4
	if tooMany {
		e.lastError = "The engine keeps stopping, so merging was turned off and your normal internet is back. " + tailFile(filepath.Join(rootDir, "data", "engine.log"), 4)
	}
	e.mu.Unlock()
	if tooMany {
		logf("%s - crashed repeatedly, giving up", why)
		killProcessByName("mihomo.exe")
		return
	}
	logf("%s - restarting", why)
	time.Sleep(2 * time.Second)
	if err := e.Start(); err != nil {
		logf("auto-restart failed: %v", err)
	}
}

// Check notices an engine that stopped answering (also covers one adopted from an earlier session).
func (e *Engine) Check() {
	e.mu.Lock()
	running := e.running
	e.mu.Unlock()
	if !running {
		return
	}
	if msg := detectConflict(); msg != "" {
		logf("stopping merging: %s", msg)
		e.Stop()
		e.mu.Lock()
		e.lastError = msg
		e.mu.Unlock()
		return
	}
	if _, ok := engineAlive(); ok {
		e.mu.Lock()
		e.apiFails = 0
		e.mu.Unlock()
		return
	}
	e.mu.Lock()
	e.apiFails++
	dead := e.apiFails >= 3
	if dead {
		e.running, e.cmd, e.apiFails = false, nil, 0
	}
	e.mu.Unlock()
	if dead {
		killProcessByName("mihomo.exe")
		e.restartAfter("engine stopped answering")
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	cmd := e.cmd
	e.running, e.cmd = false, nil
	e.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	killProcessByName("mihomo.exe")
	logf("merging stopped")
}

// Apply writes the config and hot-reloads the engine.
func (e *Engine) Apply() error {
	e.applyMu.Lock()
	defer e.applyMu.Unlock()
	if err := writeConfig(); err != nil {
		return err
	}
	syncShareFirewall()
	running, _, _, _ := e.Status()
	if !running {
		return nil
	}
	_, code, err := apiReq("PUT", "/configs?force=true", map[string]string{"path": configPath()})
	if err != nil || (code != 204 && code != 200) {
		logf("hot reload failed (code %d, %v) - restarting engine", code, err)
		e.Stop()
		time.Sleep(time.Second)
		return e.Start()
	}
	e.mu.Lock()
	e.appliedKey = activeKey()
	e.mu.Unlock()
	logf("settings applied")
	return nil
}

func tailFile(p string, n int) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
