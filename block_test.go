package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	in := `# c
0.0.0.0 ads.example.com
127.0.0.1 localhost
||tracker.test^
||bad.test/path
*.wild.example.org
plain.example.net # note
! x
::1 ip6-localhost
1.2.3.4
UPPER.Example.COM
+.plus.example.io
not a domain
`
	got := parseDomains(strings.NewReader(in))
	want := []string{"ads.example.com", "tracker.test", "wild.example.org", "plain.example.net", "upper.example.com", "plus.example.io"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRebuildAndConfig(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	if err := writeUserList("blocklist.txt", []string{"mine.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := writeUserList("allowlist.txt", []string{"ok.example.com"}); err != nil {
		t.Fatal(err)
	}
	if err := rebuildFilters(getSettings()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(rootDir, "config", "filters", "nm_block.txt"))
	if !strings.Contains(string(b), "+.mine.example.com") || !strings.Contains(string(b), "+.doubleclick.net") {
		t.Fatalf("block file wrong: %.200s", b)
	}
	a, _ := os.ReadFile(filepath.Join(rootDir, "config", "filters", "nm_allow.txt"))
	if !strings.Contains(string(a), "+.ok.example.com") || !strings.Contains(string(a), "+.www.gstatic.com") {
		t.Fatalf("allow file wrong: %s", a)
	}
	if blockedRe.FindStringSubmatch(`time="x" level=info msg="[TCP] 198.18.0.1:55696(node.exe) --> ads.doubleclick.net:443 match RuleSet(nm_block) using REJECT"`) == nil {
		t.Fatal("regex")
	}
	// the line mihomo 1.19.33 really writes for the combined rule (seen in a run of the engine)
	if m := blockedRe.FindStringSubmatch(`time="x" level=info msg="[TCP] 198.18.0.1:55696(node.exe) --> ads.doubleclick.net:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`); m == nil || m[1] != "node.exe" || m[2] != "ads.doubleclick.net:443" {
		t.Fatalf("regex for the combined rule: %v", m)
	}
	if blockedRe.MatchString(`time="x" level=info msg="[TCP] 1.1.1.1:5(git.exe) --> www.allowed.test:443 match DomainSuffix(allowed.test) using LINK1"`) {
		t.Fatal("an allowed website counted as blocked")
	}
	cfg, _ := buildConfig(getSettings())
	s := string(cfg)
	for _, w := range []string{`"AND,((RULE-SET,nm_block),(NOT,((RULE-SET,nm_allow)))),REJECT"`, `"rule-providers"`, `"log-level": "info"`} {
		if !strings.Contains(s, w) {
			t.Fatalf("config lacks %s", w)
		}
	}
	// an allowed website must not be routed by the allow list itself: it goes on to app rules and the rest
	if strings.Contains(s, `"RULE-SET,nm_allow,`) {
		t.Fatal("the allow list still decides the route")
	}
	if blk, app := strings.Index(s, "nm_block),(NOT"), strings.Index(s, "mstsc"); blk < 0 || app < 0 || blk > app {
		t.Fatalf("blocking must come before app rules (block at %d, app rule at %d)", blk, app)
	}
	// off
	st := getSettings()
	st.Block = &BlockSettings{}
	cfg, _ = buildConfig(st)
	if strings.Contains(string(cfg), "nm_block") {
		t.Fatal("blocking off still in config")
	}
}

func TestPump(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	pr, pw, _ := os.Pipe()
	lf, _ := os.Create(filepath.Join(rootDir, "e.log"))
	done := make(chan struct{})
	go func() { pumpEngineLog(pr, lf); close(done) }()
	pw.WriteString(`time="x" level=info msg="[TCP] 1.1.1.1:5(chrome.exe) --> ads.doubleclick.net:443 match RuleSet(nm_block) using REJECT"` + "\n")
	pw.WriteString(`time="x" level=info msg="[TCP] 1.1.1.1:7(chrome.exe) --> ads.doubleclick.net:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"` + "\n")
	pw.WriteString(`time="x" level=info msg="[TCP] 1.1.1.1:6(chrome.exe) --> a.com:443 match Match using FAST[LINK1]"` + "\n")
	pw.WriteString(`time="x" level=warning msg="something odd"` + "\n")
	pw.Close()
	<-done
	b, _ := os.ReadFile(filepath.Join(rootDir, "e.log"))
	if !strings.Contains(string(b), "something odd") || strings.Contains(string(b), "a.com") || strings.Contains(string(b), "doubleclick") {
		t.Fatalf("log wrong: %s", b)
	}
	today, _, recent, top := bstats.snapshot()
	if today != 2 || len(recent) != 1 || recent[0].Host != "ads.doubleclick.net" || recent[0].App != "chrome.exe" || recent[0].Count != 2 || len(top) != 1 {
		t.Fatalf("stats wrong %v %v %v", today, recent, top)
	}
}
