package main

import (
	"reflect"
	"testing"
)

// The line chosen for the VPN is used while it works; when it has no internet the VPN goes to a line that works
// (wired first), and comes back to the chosen one once it works again.
func TestVPNLeavesALineWithoutInternet(t *testing.T) {
	links := []activeLink{
		{Link: Link{Name: "Wi-Fi"}, Proxy: "LINK1", Kind: "wifi"},
		{Link: Link{Name: "USB"}, Proxy: "LINK2", Kind: "mobile"},
		{Link: Link{Name: "Ethernet"}, Proxy: "LINK3", Kind: "ethernet"},
	}
	s := Settings{VPNLink: "Wi-Fi"}
	set := func(name string, dead bool) {
		probeMu.Lock()
		probes[name] = probeState{dead: dead}
		probeMu.Unlock()
	}
	defer func() { probeMu.Lock(); probes = map[string]probeState{}; probeMu.Unlock() }()

	if got := vpnProxy(s, links, "FAST"); got != "LINK1" {
		t.Fatalf("chosen line works: got %s", got)
	}
	set("Wi-Fi", true)
	if got := vpnProxy(s, links, "FAST"); got != "LINK3" {
		t.Fatalf("Wi-Fi down: want the wired line, got %s", got)
	}
	set("Ethernet", true)
	if got := vpnProxy(s, links, "FAST"); got != "LINK2" {
		t.Fatalf("Wi-Fi and wired down: want the line that works, got %s", got)
	}
	set("USB", true)
	if got := vpnProxy(s, links, "FAST"); got != "LINK1" {
		t.Fatalf("nothing works: keep the first line, got %s", got)
	}
	set("Wi-Fi", false)
	if got := vpnProxy(s, links, "FAST"); got != "LINK1" {
		t.Fatalf("Wi-Fi back: got %s", got)
	}
	if got := vpnProxy(Settings{}, nil, "FAST"); got != "FAST" {
		t.Fatalf("no lines: got %s", got)
	}
}

func TestLostLines(t *testing.T) {
	before := map[string]bool{"Wi-Fi": true, "Ethernet": true}
	if got := lostLines(before, map[string]bool{"Ethernet": true, "USB": true}); !reflect.DeepEqual(got, []string{"Wi-Fi"}) {
		t.Fatalf("got %v", got)
	}
	if got := lostLines(nil, map[string]bool{"Ethernet": true}); len(got) != 0 {
		t.Fatalf("first start: got %v", got)
	}
}
