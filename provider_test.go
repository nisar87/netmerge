package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestChangedNetworks(t *testing.T) {
	before := []Adapter{
		{Name: "Ethernet", IP: "192.168.1.5", Gateway: "192.168.1.1"},
		{Name: "Wi-Fi", IP: "192.168.0.7", Gateway: "192.168.0.1", SSID: "Example Home"},
	}
	same := append([]Adapter(nil), before...)
	if got := changedNetworks(before, same); len(got) != 0 {
		t.Fatalf("nothing changed, got %v", got)
	}
	now := []Adapter{before[0], {Name: "Wi-Fi", IP: "192.168.0.7", Gateway: "192.168.0.1", SSID: "Example Office"}, {Name: "USB"}}
	if got := changedNetworks(before, now); !reflect.DeepEqual(got, []string{"Wi-Fi", "USB"}) {
		t.Fatalf("got %v", got)
	}
	if got := changedNetworks(before, []Adapter{{Name: "Ethernet", IP: "192.168.1.5", Gateway: "10.0.0.1"}}); !reflect.DeepEqual(got, []string{"Ethernet"}) {
		t.Fatalf("gateway change: got %v", got)
	}
}

func TestRecheckForgetsOldAddressAndAsksForALookup(t *testing.T) {
	statsMu.Lock()
	publicIPs["Wi-Fi"], publicIPs["Ethernet"] = "203.0.113.20", "203.0.113.20"
	statsMu.Unlock()
	for len(providerKick) > 0 {
		<-providerKick
	}
	recheckProviders([]string{"Wi-Fi"})
	recheckProviders([]string{"Wi-Fi"}) // a second change before the lookup must not block
	statsMu.Lock()
	_, wifi := publicIPs["Wi-Fi"]
	eth := publicIPs["Ethernet"]
	statsMu.Unlock()
	if wifi || eth != "203.0.113.20" {
		t.Fatalf("Wi-Fi should be forgotten and Ethernet kept: wifi=%v eth=%q", wifi, eth)
	}
	if len(providerKick) != 1 {
		t.Fatalf("expected one pending lookup, got %d", len(providerKick))
	}
	<-providerKick
	if got := sameProviderLines([]LinkStat{{Label: "A", Present: true, Enabled: true, PublicIP: "203.0.113.20"}, {Label: "B", Present: true, Enabled: true}}); len(got) != 0 {
		t.Fatalf("a line without a public address cannot match: %v", got)
	}
}

func TestSameProviderIsNotedOnceInActivity(t *testing.T) {
	rootDir = t.TempDir()
	lastSameProvider = ""
	count := func(sub string) int {
		n := 0
		for _, e := range recentActivity(50) {
			if strings.Contains(e.Text, sub) {
				n++
			}
		}
		return n
	}
	noteSameProvider([]string{"Wi-Fi", "LAN"})
	noteSameProvider([]string{"LAN", "Wi-Fi"}) // same group, other order: no second note
	if n := count("LAN and Wi-Fi show the same public address"); n != 1 {
		t.Fatalf("want one note, got %d", n)
	}
	noteSameProvider(nil)
	if n := count("LAN and Wi-Fi no longer show the same public address"); n != 1 {
		t.Fatalf("want a note when it clears, got %d", n)
	}
}
