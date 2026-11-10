package main

import "testing"

func TestRateCeiling(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"426.0 Mbps", 159.75e6},
		{"100 Mbps", 125e6},
		{"1 Gbps", 375e6},
		{"10 Gbps", 3.75e9},
		{"", 125e6},
		{"garbage", 125e6},
	}
	for _, c := range cases {
		if got := rateCeiling(c.in); got != c.want {
			t.Errorf("rateCeiling(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// the counter restart seen on a USB phone: about 2.7 GB "in one second" on a 426 Mbps adapter
	if 2.7e9 <= rateCeiling("426.0 Mbps") {
		t.Error("a counter restart must be above the ceiling")
	}
}

// The restart seen on the 426 Mbps USB phone on 2026-11-10: both counters came back near zero, so the 32-bit
// difference was almost 2^32 in each direction (the day then showed 4.5 GB down and 4.4 GB up for a few hundred MB).
func TestCounterRestartIsNotTraffic(t *testing.T) {
	var last, now uint32 = 212_000_000, 3_000
	if plausibleDelta(now-last, now-last, 1.0, "426.0 Mbps") {
		t.Fatal("a counter that restarted must not count as traffic")
	}
	// ordinary traffic and a real wrap of the 32-bit counter stay accepted
	if !plausibleDelta(5_000_000, 400_000, 1.0, "426.0 Mbps") {
		t.Fatal("ordinary traffic was refused")
	}
	last, now = 4_294_000_000, 2_000_000
	if !plausibleDelta(now-last, 0, 1.0, "426.0 Mbps") {
		t.Fatal("a genuine wrap of the counter was refused")
	}
}
