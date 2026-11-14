package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Diagnostics reports: with "Send diagnostics" on (the default), NetMerge sends a short report every 6 hours, and
// a crash report soon after a crash, to a database that can only be added to (Firebase Firestore, project
// netmerge-windows; see diagnostics/README.md). A report holds a random id made for this installation, the
// version, Windows and the PC model, the settings as on/off values and counts, a summary of the connections, and
// the NetMerge log since the last report with addresses, hardware addresses, device names and the Windows user
// name taken out. It never holds the websites you visit, the apps you use, or the contents of anything.

const (
	reportProject = "netmerge-windows"
	reportKey     = "AIzaSyARuUk_zgr4IHh-8wZeDejkmTQnVNoi9LM" // a Firebase web key is not a secret: it names the project; the rules decide what it may do
	reportEvery   = 6 * time.Hour
	reportLogMax  = 100000 // characters of log per report, the newest kept
)

var reportURL = "https://firestore.googleapis.com/v1/projects/" + reportProject + "/databases/(default)/documents/reports?key=" + reportKey

type reportState struct {
	Install  string `json:"install"`  // random, made once for this installation
	LastSent int64  `json:"lastSent"` // unix time of the last report that was accepted
	SentUpTo string `json:"sentUpTo"` // time stamp of the last log line sent ("2006-01-02 15:04:05")
	Fails    int    `json:"fails"`    // reports refused or not delivered in a row
}

var (
	reportMu   sync.Mutex // one report at a time
	reportWake = make(chan struct{}, 1)
)

func reportStatePath() string { return dataPath("report.json") }

func loadReportState() reportState {
	var st reportState
	if b, err := os.ReadFile(reportStatePath()); err == nil {
		if err := json.Unmarshal(b, &st); err != nil {
			logErr("read report.json", err)
		}
	}
	if st.Install == "" {
		st.Install = randomHex(16)
		saveReportState(st)
	}
	return st
}

func saveReportState(st reportState) {
	b, _ := json.Marshal(st)
	logErr("save report.json", os.WriteFile(reportStatePath(), b, 0o644))
}

// reportSoon asks for a report within a minute (after a crash).
func reportSoon() {
	select {
	case reportWake <- struct{}{}:
	default:
	}
}

func reportsOn() bool { return !getSettings().NoDiagnostics }

// reportLoop sends a crash report soon after start (one left by the last run) and a report every 6 hours.
func reportLoop() {
	time.Sleep(90 * time.Second) // let the connections settle first
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		st := loadReportState()
		_, crashErr := os.Stat(dataPath("crash.txt"))
		due := time.Since(time.Unix(st.LastSent, 0)) >= reportEvery
		if reportsOn() && (crashErr == nil || due) {
			sendReport()
		}
		select {
		case <-tick.C:
		case <-reportWake:
			time.Sleep(30 * time.Second) // the engine restarts first, so the report can get out
		}
	}
}

// sendReport sends one report now; it returns the answer's status code (0 when it could not be sent).
func sendReport() int {
	reportMu.Lock()
	defer reportMu.Unlock()
	if !reportsOn() {
		return 0
	}
	st := loadReportState()
	logText, upTo := logSince(st.SentUpTo)
	crash, _ := os.ReadFile(dataPath("crash.txt"))
	kind := "log"
	if len(crash) > 0 {
		kind = "crash"
	}
	body := reportBody(st.Install, kind, logText, scrub(string(crash)))
	code, err := postReport(body)
	if err != nil || code != 200 {
		st.Fails++
		if st.Fails <= 3 || st.Fails%10 == 0 { // a PC offline for days does not fill the log with this
			logf("diagnostics report not sent (answer %d, %v)", code, err)
		}
		saveReportState(st)
		return code
	}
	if len(crash) > 0 {
		_ = os.Remove(dataPath("crash.txt"))
	}
	st.LastSent, st.Fails = time.Now().Unix(), 0
	if upTo != "" {
		st.SentUpTo = upTo
	}
	saveReportState(st)
	logf("diagnostics report sent (%s, %d characters of log)", kind, len(logText))
	return code
}

func postReport(body []byte) (int, error) {
	req, err := http.NewRequest("POST", reportURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
	if resp.StatusCode != 200 {
		return resp.StatusCode, fmt.Errorf("%s", firstLine(string(b)))
	}
	return 200, nil
}

// reportBody builds the Firestore document. The field names must match diagnostics/firestore.rules.
func reportBody(install, kind, logText, crash string) []byte {
	f := sysFacts()
	osText := strings.TrimSpace(fmt.Sprintf("%s %s (build %s)", f["os"], f["osVersion"], f["build"]))
	device := strings.Trim(strings.Join([]string{strings.TrimSpace(f["maker"] + " " + f["model"]), f["cpu"], f["memory"], f["lang"]}, ", "), ", ")
	settingsJSON, _ := json.Marshal(reportSettings(getSettings()))
	str := func(s string, max int) map[string]any {
		if len(s) > max {
			s = s[len(s)-max:]
		}
		return map[string]any{"stringValue": s}
	}
	fields := map[string]any{
		"install":  str(install, 64),
		"kind":     str(kind, 10),
		"app":      str(appVersion, 40),
		"os":       str(osText, 200),
		"device":   str(device, 300),
		"settings": str(string(settingsJSON), 4000),
		"engine":   str(engineSummary(), 8000),
		"log":      str(logText, reportLogMax),
		"at":       map[string]any{"timestampValue": time.Now().UTC().Format(time.RFC3339)},
	}
	if crash != "" {
		fields["crash"] = str(crash, 48000)
	}
	b, _ := json.Marshal(map[string]any{"fields": fields})
	return b
}

// reportSettings is the settings as on/off values, numbers and counts: lists only as how many entries they have,
// no names, no addresses, no passwords.
func reportSettings(s Settings) map[string]any {
	raw, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	keepText := map[string]bool{"mode": true, "vpnLink": true, "quotaAction": true, "preset": true, "dnsProvider": true}
	out := map[string]any{}
	for k, v := range m {
		switch k {
		case "secret", "deviceNames", "presetBackup":
			continue
		case "links":
			var links []map[string]any
			for _, l := range s.Links {
				links = append(links, map[string]any{"name": l.Name, "enabled": l.Enabled, "weight": l.Weight, "role": l.Role, "quota": l.QuotaGB > 0})
			}
			out[k] = links
			continue
		}
		out[k] = flatten(v, keepText[k])
	}
	return out
}

func flatten(v any, keepText bool) any {
	switch t := v.(type) {
	case bool, float64, nil:
		return t
	case string:
		if keepText {
			return t
		}
		if t == "" {
			return ""
		}
		return "(set)"
	case []any:
		return len(t)
	case map[string]any:
		o := map[string]any{}
		for k, x := range t {
			switch x.(type) {
			case bool, float64:
				o[k] = x
			case []any:
				o[k] = len(x.([]any))
			}
		}
		return o
	}
	return nil
}

// engineSummary describes the connections without addresses.
func engineSummary() string {
	snap := getSnapshot()
	running, _, _, ver := engine.Status()
	var sb strings.Builder
	fmt.Fprintf(&sb, "engine=%s running=%v merging=%v conns=%d down=%.1fMbps up=%.1fMbps\n", ver, running, snap.Running, snap.Conns, snap.TotalDown*8/1e6, snap.TotalUp*8/1e6)
	if snap.Error != "" {
		fmt.Fprintf(&sb, "error: %s\n", scrub(snap.Error))
	}
	for _, l := range snap.Links {
		hs := "unknown"
		if l.Healthy != nil {
			hs = fmt.Sprint(*l.Healthy)
		}
		fmt.Fprintf(&sb, "%s: kind=%s present=%v enabled=%v healthy=%s delay=%dms rate=%s slots=%d quotaOut=%v\n",
			l.Name, l.Kind, l.Present, l.Enabled, hs, l.Delay, l.LinkRate, l.Slots, l.QuotaOut)
	}
	return sb.String()
}

// logSince returns the log lines written after the time stamp upTo (from netmerge.log.old and netmerge.log),
// scrubbed and thinned, the newest reportLogMax characters, and the time stamp of the last line.
func logSince(upTo string) (string, string) {
	var lines []string
	last := ""
	for _, p := range []string{dataPath("netmerge.log.old"), dataPath("netmerge.log")} {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		keep := false // a line without a time (the rest of a message written over several lines) goes with the line before it
		for sc.Scan() {
			ln := sc.Text()
			if stamped(ln) {
				keep = ln[:19] > upTo
				if keep {
					last = ln[:19]
				}
			}
			if keep {
				lines = append(lines, ln)
			}
		}
		f.Close()
	}
	text := thin(lines)
	if len(text) > reportLogMax {
		text = text[len(text)-reportLogMax:]
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	return scrub(text), last
}

// stamped tells whether a log line starts with its time ("2006-01-02 15:04:05").
func stamped(ln string) bool {
	if len(ln) < 19 {
		return false
	}
	for i := 0; i < 19; i++ {
		c := ln[i]
		switch i {
		case 4, 7:
			if c != '-' {
				return false
			}
		case 10:
			if c != ' ' {
				return false
			}
		case 13, 16:
			if c != ':' {
				return false
			}
		default:
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// thin writes a line that repeats the one before it (apart from the time) as a count.
func thin(lines []string) string {
	var sb strings.Builder
	prev, repeats := "", 0
	flush := func() {
		if repeats > 0 {
			fmt.Fprintf(&sb, "                     (the same %d more times)\n", repeats)
		}
		repeats = 0
	}
	for _, ln := range lines {
		msg := ln
		if len(ln) > 21 {
			msg = ln[21:]
		}
		if msg == prev {
			repeats++
			continue
		}
		flush()
		prev = msg
		sb.WriteString(ln)
		sb.WriteByte('\n')
	}
	flush()
	return sb.String()
}

var (
	reIPv4    = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.\d{1,3}\.\d{1,3}\b`)
	reIPv6    = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{1,4}:){1,7}:?[0-9a-f]{0,4}(?:%\w+)?\b`)
	reMAC     = regexp.MustCompile(`(?i)\b(?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}\b`)
	reUser    = regexp.MustCompile(`(?i)([a-z]:\\(?:users|documents and settings)\\)[^\\":\r\n]+`)
	reName    = regexp.MustCompile(`(name|host)=("[^"]*"|\S+)`)
	reTimeHMS = regexp.MustCompile(`^\d\d:\d\d(:\d\d)?$`)
)

// scrub takes out what could point at a person: the last two parts of IPv4 addresses, IPv6 and hardware
// addresses, the Windows user name in folder paths, and device names.
func scrub(s string) string {
	s = reMAC.ReplaceAllString(s, "(hw-address)")
	s = reIPv6.ReplaceAllStringFunc(s, func(m string) string {
		if reTimeHMS.MatchString(m) || strings.Count(m, ":") < 2 || !strings.ContainsAny(strings.ToLower(m), "abcdef") && !strings.Contains(m, "::") {
			return m // a time such as 15:04:05, not an address
		}
		return "(ipv6)"
	})
	s = reIPv4.ReplaceAllString(s, "$1.$2.x.x")
	s = reUser.ReplaceAllString(s, "${1}(user)")
	s = reName.ReplaceAllString(s, "$1=(name)")
	return s
}

// reportIntro tells people who used NetMerge before this feature, once, that reports are sent and where to turn
// them off (new installations see the choice in the setup guide).
func reportIntro() {
	if _, err := os.Stat(reportStatePath()); err == nil {
		return
	}
	loadReportState() // makes the installation id, so this shows once
	if s := getSettings(); s.SetupDone && !s.NoDiagnostics {
		note("info", "NetMerge now sends short diagnostics reports (no websites, apps or files) so problems get fixed without asking you for logs. Turn this off in Settings → Maintenance → Send diagnostics.")
	}
}
