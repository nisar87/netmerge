package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Moving open connections after a change.
//
// The engine decides the line for a connection when it opens, and keeps it there: changing a rule does not touch
// what is already open, so a call or a download carries on as it was. That is right for most changes, but when you
// tell NetMerge that one app should use another line, or switch a line off, you mean "now": an app that keeps one
// long connection (a Teams or Zoom call, a game, a VPN) would otherwise stay on the old line until it closes it by
// itself, which can be hours. So after such a change NetMerge closes that app's (or that line's) open connections;
// the app opens them again at once, and they follow the new rules. A call usually shows "reconnecting" for a moment.
//
// Changes of the traffic mode do not move anything: they concern every app, and closing every connection would
// interrupt every download and call on the PC.

// moveTargets lists the apps whose rule changed and the lines that were switched off.
func moveTargets(old, now Settings) (procs map[string]bool, lines map[string]bool) {
	procs, lines = map[string]bool{}, map[string]bool{}
	rules := func(s Settings) map[string]string {
		m := map[string]string{}
		for _, r := range s.AppRules {
			m[strings.ToLower(r.Process)] = r.Target
		}
		return m
	}
	a, b := rules(old), rules(now)
	for p, t := range b {
		if a[p] != t {
			procs[p] = true
		}
	}
	for p := range a {
		if _, ok := b[p]; !ok {
			procs[p] = true
		}
	}
	was := map[string]bool{}
	for _, l := range old.Links {
		was[l.Name] = l.Enabled
	}
	for _, l := range now.Links {
		if was[l.Name] && !l.Enabled {
			lines[l.Name] = true
		}
	}
	return procs, lines
}

// workingLines: the enabled lines that are present and reach the internet (and have allowance left).
func workingLines() map[string]bool {
	m := map[string]bool{}
	for _, l := range activeLinks(getSettings()) {
		if !isDead(l.Name) && !isQuotaOut(l.Name) {
			m[l.Name] = true
		}
	}
	return m
}

// lostLines: lines that worked before and no longer do (gone, unplugged, no internet).
func lostLines(before, now map[string]bool) []string {
	var out []string
	for n := range before {
		if !now[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// moveOffLines closes what is still open on lines that stopped working. Such connections cannot recover where they
// are, and an app that keeps one open and retries on it (a VPN that is "connecting", a call) would otherwise stay
// stuck on the dead line; closed, the app opens it again on a line that works.
func moveOffLines(lines []string, started time.Time) {
	set := map[string]bool{}
	for _, l := range lines {
		set[l] = true
	}
	closeOpen(getSettings(), nil, set, started, "it stopped working")
}

// moveOffSwitched closes what is open on lines that joined another network (other router, other Wi-Fi). Those
// connections were made through the old network: the other side no longer reaches them, and an app holding one
// would wait for a timeout (a VPN or call can wait minutes). Closed, they reopen at once through the new network.
func moveOffSwitched(lines []string) {
	if running, _, _, _ := engine.Status(); !running {
		return
	}
	set := map[string]bool{}
	for _, l := range lines {
		set[l] = true
	}
	closeOpen(getSettings(), nil, set, time.Now(), "it joined another network")
}

// moveAfterChange closes the open connections of the apps whose rule changed and of the lines switched off.
// It runs after the engine has the new settings, so what reopens follows them.
func moveAfterChange(old, now Settings, started time.Time) {
	procs, lines := moveTargets(old, now)
	if len(procs) == 0 && len(lines) == 0 {
		return
	}
	closeOpen(now, procs, lines, started, "the new settings")
}

// closeOpen closes the open connections of the given apps and of the given lines.
func closeOpen(now Settings, procs, lines map[string]bool, started time.Time, reason string) {
	b, code, err := apiReq("GET", "/connections", nil)
	if err != nil || code != 200 {
		logErr("list the open connections to move them", err)
		return
	}
	var list connList
	if logErr("read the open connections to move them", json.Unmarshal(b, &list)) != nil {
		return
	}
	moved := map[string]int{}
	for _, c := range list.Connections {
		proc := strings.ToLower(c.Metadata.Process)
		line := ""
		if len(c.Chains) > 0 {
			if line = proxyToLink(now, c.Chains[0]); line == "" && c.Chains[0] == "DIRECT" {
				line = "bypass"
			}
		}
		why := ""
		switch {
		case proc != "" && procs[proc]:
			why = proc
		case line != "" && lines[line]:
			why = "line " + line
		default:
			continue
		}
		if _, code, err := apiReq("DELETE", "/connections/"+c.ID, nil); err == nil && code < 300 {
			moved[why]++
		}
	}
	for why, n := range moved {
		logf("moved %d open connection(s) of %s because of %s (%d ms)", n, why, reason, time.Since(started).Milliseconds())
	}
}
