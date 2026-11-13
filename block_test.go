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
	// the line the engine writes for the combined rule
	if m := blockedRe.FindStringSubmatch(`time="x" level=info msg="[TCP] 198.18.0.1:55696(node.exe) --> ads.doubleclick.net:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`); m == nil || blockedApp(m[1], m[2]) != "node.exe" || m[3] != "ads.doubleclick.net:443" {
		t.Fatalf("regex for the combined rule: %v", m)
	}
	// lines copied from engine.log on the real PC: a shared phone (no program name) over TCP and UDP, and a PC program
	for _, c := range []struct{ line, app, host string }{
		{`time="2026-11-12T03:12:11.207397100+05:00" level=info msg="[TCP] 192.168.1.107:60372 --> 172.217.117.4:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`, "Shared device (192.168.1.107)", "172.217.117.4:443"},
		{`time="2026-11-12T03:09:06.933493200+05:00" level=info msg="[UDP] 192.168.1.107:36424 --> 142.251.37.130:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`, "Shared device (192.168.1.107)", "142.251.37.130:443"},
		{`time="2026-11-12T03:12:16.391114200+05:00" level=info msg="[TCP] 198.18.0.1:61047(msedge.exe) --> 51.11.192.52:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`, "msedge.exe", "51.11.192.52:443"},
		{`time="x" level=info msg="[TCP] 198.18.0.1:61050 --> ads.example.com:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"`, "", "ads.example.com:443"},
	} {
		m := blockedRe.FindStringSubmatch(c.line)
		if m == nil || blockedApp(m[1], m[2]) != c.app || m[3] != c.host {
			t.Fatalf("not counted right: %v for %s", m, c.line)
		}
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
	pw.WriteString(`time="x" level=info msg="[TCP] 192.168.1.107:60372 --> 172.217.117.4:443 match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using REJECT"` + "\n")
	pw.WriteString(`time="x" level=info msg="[TCP] 1.1.1.1:6(chrome.exe) --> a.com:443 match Match using FAST[LINK1]"` + "\n")
	pw.WriteString(`time="x" level=warning msg="something odd"` + "\n")
	pw.Close()
	<-done
	b, _ := os.ReadFile(filepath.Join(rootDir, "e.log"))
	if !strings.Contains(string(b), "something odd") || strings.Contains(string(b), "a.com") || strings.Contains(string(b), "doubleclick") {
		t.Fatalf("log wrong: %s", b)
	}
	today, _, recent, top := bstats.snapshot()
	if today != 3 || len(recent) != 2 || recent[0].Host != "ads.doubleclick.net" || recent[0].App != "chrome.exe" || recent[0].Count != 2 || recent[1].App != "Shared device (192.168.1.107)" || len(top) != 2 {
		t.Fatalf("stats wrong %v %v %v", today, recent, top)
	}
}
