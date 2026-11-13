package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// What NetMerge writes to its log beyond the plain logf lines: problems with the place in our code they come from,
// crashes kept for the next start, why the last run ended, and frequent events counted once a minute instead of
// listed one by one. The log never holds website names, addresses typed by the user, or file contents; the
// diagnostics report scrubs addresses and names that Windows itself puts in error messages (see report.go).

// where names the first places in our own code that led here (skip 1 = the caller of where), e.g.
// "settings.go:112 < api.go:140", so a log line says where a problem was met without a whole stack.
func where(skip int) string {
	pcs := make([]uintptr, 16)
	n := runtime.Callers(skip+1, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var out []string
	for len(out) < 3 {
		f, more := frames.Next()
		if strings.HasPrefix(f.Function, "main.") || strings.HasPrefix(f.Function, "netmerge.") || strings.HasPrefix(f.Function, "netmerge/") {
			if !strings.HasSuffix(f.File, "eventlog.go") {
				out = append(out, fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line))
			}
		}
		if !more {
			break
		}
	}
	return strings.Join(out, " < ")
}

// firstLine keeps an error short: Windows tools answer with several lines of help text.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i]) + " …"
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// logErr writes "problem: <what was being done>: <error> (where)" when err is not nil, and returns err.
// It is for failures that used to be dropped without a word.
func logErr(what string, err error) error {
	if err != nil {
		logf("problem: %s: %s (%s)", what, firstLine(err.Error()), where(2))
	}
	return err
}

// ---- frequent events: counted, written once a minute ----

var (
	countMu sync.Mutex
	counts  = map[string]int{}
)

// count adds one to a counter such as "tcp.Wi-Fi.timeout"; countsLoop writes all counters once a minute.
func count(key string) {
	countMu.Lock()
	counts[key]++
	countMu.Unlock()
}

func flushCounts() {
	countMu.Lock()
	if len(counts) == 0 {
		countMu.Unlock()
		return
	}
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, counts[k]))
	}
	counts = map[string]int{}
	countMu.Unlock()
	logf("counts in the last minute: %s", strings.Join(parts, " "))
}

func countsLoop() {
	for range time.Tick(time.Minute) {
		flushCounts()
		touchRunMarker()
	}
}

// failClass sorts a connection error into a few words, for counting.
func failClass(err string) string {
	e := strings.ToLower(err)
	switch {
	case strings.Contains(e, "timeout"), strings.Contains(e, "timed out"), strings.Contains(e, "deadline"), strings.Contains(e, "did not connect in time"):
		return "timeout"
	case strings.Contains(e, "refused"):
		return "refused"
	case strings.Contains(e, "reset"), strings.Contains(e, "forcibly closed"), strings.Contains(e, "aborted"):
		return "reset"
	case strings.Contains(e, "unreachable"), strings.Contains(e, "no route"):
		return "unreachable"
	case strings.Contains(e, "cannot find the address"), strings.Contains(e, "no such host"), strings.Contains(e, "lookup"):
		return "dns"
	case strings.Contains(e, "no line"), strings.Contains(e, "no proxy"), strings.Contains(e, "no usable"):
		return "noline"
	case strings.Contains(e, "eof"), strings.Contains(e, "closed"):
		return "closed"
	}
	return "other"
}

// ---- crashes and how the last run ended ----

func dataPath(name string) string { return filepath.Join(rootDir, "data", name) }

// pendingCrash keeps a crash for the next diagnostics report (crash.txt); a later crash is added below it.
func pendingCrash(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	p := dataPath("crash.txt")
	old, _ := os.ReadFile(p)
	b := append(old, []byte(time.Now().Format("2006-01-02 15:04:05")+"\n"+text+"\n\n")...)
	if len(b) > 48000 {
		b = b[len(b)-48000:]
	}
	_ = logErr("keep the crash report", os.WriteFile(p, b, 0o644))
	reportSoon()
}

// stackOfOurs is the stack of the current goroutine, at most 40 lines (a panic deep in the standard library is
// still traced back to our code).
func stackOfOurs() string {
	buf := make([]byte, 16<<10)
	buf = buf[:runtime.Stack(buf, false)]
	lines := strings.Split(string(buf), "\n")
	if len(lines) > 40 {
		lines = lines[:40]
	}
	return strings.Join(lines, "\n")
}

// The run marker exists while NetMerge runs and is removed when it quits normally. Found at start, it means the
// last run did not end by quitting: Windows shut down or restarted, the power went off, or the program crashed or
// was ended from Task Manager. It is touched once a minute, so the time it was last alive is known.
type runMarker struct {
	Version string `json:"version"`
	Started int64  `json:"started"`
	Alive   int64  `json:"alive"`
	Pid     int    `json:"pid"`
}

var runStarted = time.Now()

func touchRunMarker() {
	b, _ := json.Marshal(runMarker{appVersion, runStarted.Unix(), time.Now().Unix(), os.Getpid()})
	_ = os.WriteFile(dataPath("running.json"), b, 0o644)
}

func removeRunMarker() { _ = os.Remove(dataPath("running.json")) }

// checkLastRun writes how the last run ended, and keeps a Go crash report it left behind for the next report.
func checkLastRun() {
	if b, err := os.ReadFile(dataPath("running.json")); err == nil {
		var m runMarker
		_ = json.Unmarshal(b, &m)
		alive := time.Unix(m.Alive, 0)
		boot := bootTime()
		ran := alive.Sub(time.Unix(m.Started, 0)).Round(time.Minute)
		switch {
		case !boot.IsZero() && boot.After(alive):
			logf("last run (%s, ran %v) ended when Windows shut down or restarted (last seen %s, Windows started %s)",
				m.Version, ran, alive.Format("2006-01-02 15:04"), boot.Format("2006-01-02 15:04"))
		default:
			logf("last run (%s, ran %v) ended unexpectedly at about %s: it crashed, or it was ended from Task Manager",
				m.Version, ran, alive.Format("2006-01-02 15:04"))
			count("unexpected-end")
		}
	} else {
		logf("last run ended normally (or this is the first run)")
	}
	// a hard crash of the Go runtime is written to go-crash.log after the "--- start" line of that run
	if b, err := os.ReadFile(dataPath("go-crash.log")); err == nil {
		s := string(b)
		if i := strings.LastIndex(s, "--- start"); i >= 0 {
			if j := strings.Index(s[i:], "\n"); j >= 0 {
				if crash := strings.TrimSpace(s[i+j:]); crash != "" {
					logf("the last run crashed: %s", firstLine(crash))
					pendingCrash("NetMerge crashed (Go runtime):\n" + crash)
				}
			}
		}
	}
	// the updater script writes what it did; an update that went wrong shows here
	if b, err := os.ReadFile(dataPath("update.log")); err == nil {
		for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if ln = strings.TrimSpace(ln); ln != "" {
				logf("updater: %s", ln)
			}
		}
		_ = os.Remove(dataPath("update.log"))
	}
	touchRunMarker()
}

// ---- start-up facts ----

var (
	sysInfoMu sync.Mutex
	sysInfo   = map[string]string{}
)

func sysFacts() map[string]string {
	sysInfoMu.Lock()
	defer sysInfoMu.Unlock()
	c := make(map[string]string, len(sysInfo))
	for k, v := range sysInfo {
		c[k] = v
	}
	return c
}

// logStartFacts writes the version, Windows and the PC once at start (read in the background: Windows takes a
// second or two to answer).
func logStartFacts(background bool) {
	logf("NetMerge %s starting: %s/%s, %d processors, Go %s, background=%v, administrator=%v",
		appVersion, runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.Version(), background, isAdmin())
	go safely("system facts", func() {
		f := readSysFacts()
		sysInfoMu.Lock()
		sysInfo = f
		sysInfoMu.Unlock()
		logf("system: %s %s (build %s), %s %s, %s, memory %s", f["os"], f["osVersion"], f["build"], f["maker"], f["model"], f["cpu"], f["memory"])
	})
}

// clientError takes an error the window met (a script error, a refused request) and writes it to the log, at
// most 30 a minute so a page stuck in a loop cannot fill the log.
var (
	clientMu    sync.Mutex
	clientMin   time.Time
	clientCount int
)

func clientError(kind, msg, src string, line int) {
	clientMu.Lock()
	if time.Since(clientMin) > time.Minute {
		clientMin, clientCount = time.Now(), 0
	}
	clientCount++
	n := clientCount
	clientMu.Unlock()
	if n > 30 {
		count("window." + kind + ".dropped")
		return
	}
	if len(msg) > 400 {
		msg = msg[:400] + "…"
	}
	src = filepath.Base(strings.SplitN(src, "?", 2)[0])
	logf("window %s: %s (%s:%d)", kind, strings.ReplaceAll(msg, "\n", " | "), src, line)
}

// logSettingsChange writes which settings changed, as the report shows them (on/off, numbers, counts).
func logSettingsChange(old, now Settings) {
	a, b := reportSettings(old), reportSettings(now)
	var changed []string
	for k, v := range b {
		x, _ := json.Marshal(a[k])
		y, _ := json.Marshal(v)
		if string(x) != string(y) {
			changed = append(changed, fmt.Sprintf("%s %s -> %s", k, x, y))
		}
	}
	if len(changed) > 0 {
		sort.Strings(changed)
		logf("settings changed: %s", strings.Join(changed, "; "))
	}
}

// ---- the engine's own output ----

var (
	engRawMu sync.Mutex
	engRaw   []string // the engine's last lines that are not ordinary log lines (a crash report is written this way)
	reEngLvl = regexp.MustCompile(`level=(\w+) msg="(.*)"$`)
	reDial   = regexp.MustCompile(`^\[(TCP|UDP)\] dial (\S+) .*error: (.*)$`)
	reProto  = regexp.MustCompile(`^\[(TCP|UDP)\] `)
)

// noteEngineLine looks at each line the engine writes (it runs for every connection, so it stays cheap):
// failed connections are counted per line and cause (never with the website), the engine's own warnings and
// errors go to NetMerge's log, and anything that is not a log line (a crash) is kept for a crash report.
func noteEngineLine(line string) {
	if !strings.HasPrefix(line, "time=") {
		engRawMu.Lock()
		engRaw = append(engRaw, line)
		if len(engRaw) > 120 {
			engRaw = engRaw[len(engRaw)-120:]
		}
		engRawMu.Unlock()
		return
	}
	if !strings.Contains(line, "level=warning") && !strings.Contains(line, "level=error") {
		return
	}
	m := reEngLvl.FindStringSubmatch(line)
	if m == nil {
		return
	}
	msg := strings.ReplaceAll(m[2], `\"`, `"`)
	if d := reDial.FindStringSubmatch(msg); d != nil {
		lineName := d[2]
		if i := strings.IndexByte(lineName, '['); i >= 0 {
			lineName = strings.TrimSuffix(lineName[i+1:], "]")
		}
		count(strings.ToLower(d[1]) + "." + lineName + "." + failClass(d[3]))
		return
	}
	if p := reProto.FindStringSubmatch(msg); p != nil {
		count(strings.ToLower(p[1]) + ".fail." + failClass(msg))
		return
	}
	logf("engine %s: %s", m[1], firstLine(msg))
}

// engineCrashText is what the engine wrote outside its log lines (its crash report), emptied once read.
func engineCrashText() string {
	engRawMu.Lock()
	defer engRawMu.Unlock()
	s := strings.TrimSpace(strings.Join(engRaw, "\n"))
	engRaw = nil
	return s
}
