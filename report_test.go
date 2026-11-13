package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScrubTakesOutWhatPointsAtAPerson(t *testing.T) {
	in := `2026-11-13 10:00:00  device 192.168.1.37: mac=a4:5e:60:12:34:56 name="Ali's iPhone"
2026-11-13 10:00:01  problem: save settings: open C:\Users\Nisar Ahmad\NetMerge\data\settings.json.tmp: denied (settings.go:112)
2026-11-13 10:00:02  public 39.45.123.200 ipv6 2400:adc5:1a3::12 fe80::1%12 host=desktop-ab12
2026-11-13 10:00:03  engine 3.18.1 at 15:04:05 build 26100.2314`
	out := scrub(in)
	for _, bad := range []string{"192.168.1.37", "a4:5e:60", "Ali", "Nisar", "39.45.123", "2400:adc5", "fe80::1", "desktop-ab12"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q still in the scrubbed text:\n%s", bad, out)
		}
	}
	for _, keep := range []string{"192.168.x.x", "39.45.x.x", `C:\Users\(user)\NetMerge`, "3.18.1", "15:04:05", "26100.2314", "settings.go:112", "2026-11-13 10:00:03"} {
		if !strings.Contains(out, keep) {
			t.Errorf("%q missing from the scrubbed text:\n%s", keep, out)
		}
	}
}

func TestThinCountsRepeatedLines(t *testing.T) {
	out := thin([]string{
		"2026-11-13 10:00:00  a",
		"2026-11-13 10:00:01  b",
		"2026-11-13 10:00:02  b",
		"2026-11-13 10:00:03  b",
		"2026-11-13 10:00:04  c",
	})
	if strings.Count(out, "  b\n") != 1 || !strings.Contains(out, "(the same 2 more times)") || !strings.HasSuffix(out, "  c\n") {
		t.Fatalf("thinned:\n%s", out)
	}
}

func TestReportSettingsHoldNoNamesOrLists(t *testing.T) {
	s := defaultSettings()
	s.Secret = "s3cret"
	s.Links = []Link{{Name: "Wi-Fi", Label: "Ali's phone", Enabled: true, Weight: 2, QuotaGB: 10}}
	s.AppRules = []AppRule{{Process: "secretapp.exe", Target: "fast"}}
	s.BypassIPs = []string{"10.1.2.3"}
	s.DeviceNames = map[string]string{"aa": "Mum's laptop"}
	s.Share = &ShareSettings{Enabled: true, User: "bob", Pass: "hunter2"}
	b, _ := json.Marshal(reportSettings(s))
	text := string(b)
	for _, bad := range []string{"s3cret", "Ali", "secretapp", "10.1.2.3", "Mum", "bob", "hunter2"} {
		if strings.Contains(text, bad) {
			t.Errorf("%q in the report settings: %s", bad, text)
		}
	}
	if !strings.Contains(text, `"appRules":1`) || !strings.Contains(text, `"name":"Wi-Fi"`) {
		t.Errorf("counts or line names missing: %s", text)
	}
}

// the fields must be the ones diagnostics/firestore.rules allows, or every report is refused
func TestReportFieldsMatchTheRules(t *testing.T) {
	rules, err := os.ReadFile("diagnostics/firestore.rules")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if err := json.Unmarshal(reportBody("id", "crash", "log", "crash text"), &doc); err != nil {
		t.Fatal(err)
	}
	for k := range doc.Fields {
		if !strings.Contains(string(rules), "'"+k+"'") {
			t.Errorf("field %q is not allowed by the rules", k)
		}
	}
}

func TestSendReportSendsTheNewLogOnceAndTheCrash(t *testing.T) {
	old := rootDir
	rootDir = t.TempDir()
	defer func() { rootDir = old }()
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	loadSettings()
	now := time.Now()
	line := func(d time.Duration, msg string) string {
		return now.Add(d).Format("2006-01-02 15:04:05") + "  " + msg + "\n"
	}
	_ = os.WriteFile(dataPath("netmerge.log"), []byte(line(-3*time.Second, "first")+line(-2*time.Second, "device 10.0.0.5: name=\"Phone\"")), 0o644)
	_ = os.WriteFile(dataPath("crash.txt"), []byte("panic: boom"), 0o644)

	var got []map[string]map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var doc struct {
			Fields map[string]map[string]string `json:"fields"`
		}
		_ = json.Unmarshal(b, &doc)
		got = append(got, doc.Fields)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	oldURL := reportURL
	reportURL = srv.URL
	defer func() { reportURL = oldURL }()

	if code := sendReport(); code != 200 {
		t.Fatalf("answer %d", code)
	}
	r := got[0]
	if r["kind"]["stringValue"] != "crash" || r["crash"]["stringValue"] == "" || !strings.Contains(r["log"]["stringValue"], "first") {
		t.Fatalf("first report: %v", r)
	}
	if strings.Contains(r["log"]["stringValue"], "10.0.0.5") || strings.Contains(r["log"]["stringValue"], "Phone") {
		t.Fatalf("not scrubbed: %s", r["log"]["stringValue"])
	}
	if _, err := os.Stat(dataPath("crash.txt")); err == nil {
		t.Fatal("crash.txt should be gone once sent")
	}
	// the next report starts after what was sent (the "report sent" line itself is new)
	sendReport()
	if l := got[1]["log"]["stringValue"]; strings.Contains(l, "first") || got[1]["kind"]["stringValue"] != "log" {
		t.Fatalf("second report repeats the log or is not a log report: %v", got[1])
	}
	// turned off: nothing is sent
	updateSettings(func(s *Settings) { s.NoDiagnostics = true })
	if sendReport() != 0 || len(got) != 2 {
		t.Fatal("a report was sent with diagnostics off")
	}
}

func TestEngineLinesAreCountedWithoutWebsites(t *testing.T) {
	countMu.Lock()
	counts = map[string]int{}
	countMu.Unlock()
	noteEngineLine(`time="x" level=warning msg="[TCP] dial AUTO[LINK2] (match Match/) 192.168.1.5:5000(chrome.exe) --> secret-site.com:443 error: dial tcp 1.2.3.4:443: i/o timeout"`)
	noteEngineLine(`time="x" level=warning msg="[UDP] dial LINK1 (match Match/) 192.168.1.5:5000(app.exe) --> other.com:443 error: no line carries UDP"`)
	noteEngineLine(`time="x" level=info msg="[TCP] 192.168.1.5:5000 --> site.com:443 match Match using LINK1"`)
	noteEngineLine(`panic: runtime error: index out of range`)
	countMu.Lock()
	got := map[string]int{}
	for k, v := range counts {
		got[k] = v
	}
	countMu.Unlock()
	if got["tcp.LINK2.timeout"] != 1 || got["udp.LINK1.noline"] != 1 || len(got) != 2 {
		t.Fatalf("counts: %v", got)
	}
	if c := engineCrashText(); !strings.Contains(c, "index out of range") {
		t.Fatalf("crash text: %q", c)
	}
}

func TestWhereNamesOurCode(t *testing.T) {
	w := func() string { return where(1) }()
	if !strings.Contains(w, "report_test.go:") {
		t.Fatalf("where: %q", w)
	}
}

// A real report to the real database, run in CI (NETMERGE_REPORT_LIVE=1): it must be accepted, so a change to the
// rules or to the fields cannot quietly stop every report. The daily reader ignores installs named "ci-test".
func TestReportAcceptedByFirestore(t *testing.T) {
	if os.Getenv("NETMERGE_REPORT_LIVE") == "" {
		t.Skip("set NETMERGE_REPORT_LIVE=1 to send a test report")
	}
	body := reportBody("ci-test-"+randomHex(4), "log", "2026-01-01 00:00:00  CI test report\n", "")
	code, err := postReport(body)
	if code != 200 {
		t.Fatalf("report refused: %d %v", code, err)
	}
	// a field the rules do not allow must be refused
	var doc map[string]map[string]any
	_ = json.Unmarshal(body, &doc)
	doc["fields"]["extra"] = map[string]any{"stringValue": "x"}
	bad, _ := json.Marshal(doc)
	if code, _ := postReport(bad); code != 403 {
		t.Fatalf("a report with an unknown field got %d, want 403", code)
	}
}
