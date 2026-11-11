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
	startMu       sync.Mutex // one start at a time; never held by Status, so the window never waits for a start
	cmd           *exec.Cmd
	own           bool // the running engine is NetMerge's own (NetMerge.exe -engine), not mihomo
	fellBack      bool // NetMerge's own engine was chosen but would not start, so mihomo runs
	running       bool
	startedAt     time.Time
	lastError     string
	version       string
	restarts      int
	restartWindow time.Time
	appliedKey    string
	reloads       int // config reloads; a reload resets the engine's selectors, so the dispatcher sets them again
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

type procOf struct{ pid int }

func (p procOf) Pid() int { return p.pid }

// Own tells whether NetMerge's own engine is the one running, and whether it was chosen but mihomo runs instead.
func (e *Engine) Own() (own, fellBack bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.own && e.running, e.fellBack
}

func (e *Engine) AppliedKey() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.appliedKey
}

// LoadID changes whenever the engine loaded its settings again (start or reload).
func (e *Engine) LoadID() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return fmt.Sprintf("%s|%d|%d", e.appliedKey, e.startedAt.Unix(), e.reloads)
}

// Adopt an engine left running by a previous NetMerge session.
func (e *Engine) Adopt() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := engineAlive(); ok {
		e.running, e.version, e.startedAt = true, v, time.Now()
		e.own = recordedIsOwn() || strings.HasPrefix(v, "nmengine")
		logf("re-attached to running engine %s", v)
		return
	}
	// an engine of ours we cannot talk to (left over from an old session) - remove it; other programs' are left alone
	stopOwnEngines()
}

// engineCommand is the engine program to run: NetMerge's own (this same exe with -engine) or mihomo.
func engineCommand(own bool) (string, []string) {
	if own {
		exe, _ := os.Executable()
		return exe, []string{"-engine", "-d", configDir()}
	}
	return mihomoExe(), []string{"-d", configDir()}
}

func (e *Engine) setError(msg string) {
	e.mu.Lock()
	e.lastError = msg
	e.mu.Unlock()
}

// Start starts the chosen engine. The lock that Status uses is only taken for a moment, so the window keeps
// answering while the engine starts (it used to wait up to 15 seconds).
func (e *Engine) Start() error {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	e.mu.Lock()
	running := e.running
	e.mu.Unlock()
	if running {
		return nil
	}
	own := getSettings().Engine == "netmerge"
	err := e.startWith(own)
	if err != nil && own {
		// NetMerge's own engine is still being tested: if it will not start, use mihomo so the internet keeps working
		logf("NetMerge engine did not start (%v) - using mihomo instead", err)
		note("warn", "NetMerge's own engine did not start, so merging uses the standard engine (mihomo). See the engine log for why.")
		if err2 := e.startWith(false); err2 == nil {
			e.mu.Lock()
			e.fellBack = true
			e.mu.Unlock()
			return nil
		}
	}
	return err
}

func (e *Engine) startWith(own bool) error {
	if !own {
		if _, err := os.Stat(mihomoExe()); err != nil {
			msg := "The network engine is not installed yet. NetMerge downloads it by itself - try again in a minute."
			e.setError(msg)
			if st, _, _ := engineDownloadState(); st == "" {
				go safely("engine download", downloadEngine)
			}
			return fmt.Errorf("%s", msg)
		}
	}
	if msg := detectConflictNow(); msg != "" {
		e.setError(msg)
		logf("not starting: %s", msg)
		return fmt.Errorf("%s", msg)
	}
	links := activeLinks(getSettings())
	if len(links) == 0 {
		msg := "No internet connection found. Plug in or connect at least one, then try again."
		e.setError(msg)
		return fmt.Errorf("%s", msg)
	}
	stopOwnEngines()
	if err := writeConfig(); err != nil {
		e.setError("Could not write config: " + err.Error())
		return err
	}
	syncShareFirewall()
	logPath := filepath.Join(rootDir, "data", "engine.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 8<<20 {
		_ = os.Rename(logPath, logPath+".old")
	}
	lf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	exe, args := engineCommand(own)
	cmd := hiddenCmd(exe, args...)
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
		e.setError("Could not start engine: " + err.Error())
		return err
	}
	if perr == nil {
		pw.Close() // the engine holds its own copy; we read until it exits
		go pumpEngineLog(pr, lf)
	}
	rememberEngine(cmd.Process.Pid, exe)
	e.mu.Lock()
	e.cmd, e.own = cmd, own
	e.mu.Unlock()
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
			msg := "The engine stopped right after starting (" + fmt.Sprint(err) + "). " + tailFile(logPath, 6)
			e.mu.Lock()
			e.lastError, e.cmd = msg, nil
			e.mu.Unlock()
			forgetEngine()
			dnsStartFailed(msg)
			logf("engine failed: %s", msg)
			return fmt.Errorf("%s", msg)
		default:
		}
		if v, ok := engineAlive(); ok {
			e.mu.Lock()
			e.running, e.version, e.startedAt, e.lastError, e.apiFails, e.fellBack = true, v, time.Now(), "", 0, false
			e.appliedKey = keyOf(links)
			e.mu.Unlock()
			logf("engine %s started with %d connection(s)", v, len(links))
			note("good", "Merging started with %d connection(s)", len(links))
			go e.watch(cmd, exited)
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	msg := "The engine did not respond. " + tailFile(logPath, 6)
	e.mu.Lock()
	e.cmd, e.lastError = nil, msg
	e.mu.Unlock()
	forgetEngine()
	dnsStartFailed(msg)
	return fmt.Errorf("%s", msg)
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
		note("bad", "The network engine stopped unexpectedly. NetMerge is restarting it.")
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
		stopOwnEngines()
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
		stopOwnEngines()
		e.restartAfter("engine stopped answering")
	}
}

func (e *Engine) Stop() {
	e.mu.Lock()
	cmd, own := e.cmd, e.own
	was := e.running
	e.running, e.cmd = false, nil
	e.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		stopEngineProcess(procOf{cmd.Process.Pid}, own)
	}
	stopOwnEngines()
	forgetEngine()
	logf("merging stopped")
	if was {
		note("info", "Merging stopped")
	}
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
	e.reloads++
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
