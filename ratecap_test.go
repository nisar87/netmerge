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
