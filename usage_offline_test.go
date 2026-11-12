package main

import (
	"testing"
	"time"
)

func TestOfflineUsage(t *testing.T) {
	boot := time.Unix(1_700_000_000, 0)
	now := boot.Add(5 * time.Hour)
	snap := ctrSnap{Boot: boot.Unix() + 3, At: now.Add(-2 * time.Hour).Unix(), Links: map[string]ctrLink{
		"Wi-Fi":      {Idx: 7, Desc: "Wireless", Rx: 1000, Tx: 500},
		"Ethernet":   {Idx: 11, Desc: "Realtek", Rx: 9000, Tx: 9000}, // reset since: counter smaller now
		"Cellular":   {Idx: 14, Desc: "Phone", Rx: 0, Tx: 0},         // now on another adapter index
		"Ethernet 2": {Idx: 15, Desc: "USB", Rx: 0, Tx: 0},           // impossible jump
	}}
	adapters := map[string]Adapter{
		"Wi-Fi": {Name: "Wi-Fi", Index: 7, Desc: "Wireless"}, "Ethernet": {Name: "Ethernet", Index: 11, Desc: "Realtek"},
		"Cellular": {Name: "Cellular", Index: 20, Desc: "Phone"}, "Ethernet 2": {Name: "Ethernet 2", Index: 15, Desc: "USB"},
	}
	c64 := map[int][2]uint64{7: {6_000_000_000, 700}, 11: {10, 10}, 20: {50, 50}, 15: {1 << 62, 0}}
	got := offlineUsage(snap, boot, now, adapters, c64)
	if len(got) != 1 || got["Wi-Fi"] != [2]uint64{6_000_000_000 - 1000, 200} {
		t.Fatalf("got %v", got)
	}
	// Windows restarted in between: nothing is counted
	if got := offlineUsage(snap, boot.Add(time.Hour), now, adapters, c64); len(got) != 0 {
		t.Fatalf("after a restart got %v", got)
	}
}
