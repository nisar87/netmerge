package main

import (
	"strings"
	"testing"
)

func TestMoveTargets(t *testing.T) {
	old := Settings{
		AppRules: []AppRule{{Process: "ms-teams.exe", Target: "link:Wi-Fi"}, {Process: "steam.exe", Target: "fast"}, {Process: "zoom.exe", Target: "bypass"}},
		Links:    []Link{{Name: "Ethernet", Enabled: true}, {Name: "Wi-Fi", Enabled: true}, {Name: "Phone", Enabled: false}},
	}
	now := Settings{
		AppRules: []AppRule{{Process: "MS-Teams.exe", Target: "link:Ethernet"}, {Process: "steam.exe", Target: "fast"}, {Process: "chrome.exe", Target: "stable"}},
		Links:    []Link{{Name: "Ethernet", Enabled: true}, {Name: "Wi-Fi", Enabled: false}, {Name: "Phone", Enabled: true}},
	}
	procs, lines := moveTargets(old, now)
	// changed, added and removed rules move; an unchanged rule does not
	for _, p := range []string{"ms-teams.exe", "chrome.exe", "zoom.exe"} {
		if !procs[p] {
			t.Errorf("%s should move", p)
		}
	}
	if procs["steam.exe"] {
		t.Error("steam.exe's rule did not change")
	}
	// a line switched off moves; a line switched on does not
	if !lines["Wi-Fi"] || lines["Phone"] || lines["Ethernet"] {
		t.Errorf("lines: %v", lines)
	}
	if p, l := moveTargets(now, now); len(p)+len(l) != 0 {
		t.Error("no change should move nothing")
	}
}

func TestCleanAppName(t *testing.T) {
	cases := map[[2]string]string{
		{"Microsoft Teams", "ms-teams.exe"}:  "Microsoft Teams",
		{"  Google   Chrome ", "chrome.exe"}: "Google Chrome",
		{"chrome", "chrome.exe"}:             "",
		{"Application", "foo.exe"}:           "",
		{"", "foo.exe"}:                      "",
	}
	for in, want := range cases {
		if got := cleanAppName(in[0], in[1]); got != want {
			t.Errorf("cleanAppName(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
	}
	if got := cleanAppName(strings.Repeat("x", 80), "a.exe"); len([]rune(got)) != 48 {
		t.Errorf("long names are cut: %q", got)
	}
}
