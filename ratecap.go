package main

import (
	"strconv"
	"strings"
)

// A network adapter's byte counter can restart from zero when the adapter is unplugged and plugged in again (a USB
// phone, a Wi-Fi stick). Read as an ordinary 32-bit counter that looks like billions of bytes in one second.
// Such a reading was once taken for the line's real speed, learned as its capacity and saved, which left every
// other line looking "too slow to help". A reading far above what the adapter can carry is therefore ignored.

// linkRateBytes turns the adapter's reported link speed ("426.0 Mbps", "1 Gbps") into bytes per second; 0 if unknown.
func linkRateBytes(rate string) float64 {
	f := strings.Fields(strings.TrimSpace(rate))
	if len(f) < 2 {
		return 0
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v <= 0 {
		return 0
	}
	switch strings.ToLower(f[1]) {
	case "kbps":
		return v * 1e3 / 8
	case "mbps":
		return v * 1e6 / 8
	case "gbps":
		return v * 1e9 / 8
	}
	return 0
}

// rateCeiling is the most bytes per second a line may plausibly carry: three times what the adapter reports, and
// never less than 1 Gbit/s (adapters often report a link speed lower than what they really do).
func rateCeiling(linkRate string) float64 {
	c := 3 * linkRateBytes(linkRate)
	if c < 125e6 {
		c = 125e6
	}
	return c
}

// plausibleDelta: can this many bytes (in / out) have crossed the adapter in dt seconds?
func plausibleDelta(dr, dtx uint32, dt float64, linkRate string) bool {
	if dt <= 0 {
		return true
	}
	c := rateCeiling(linkRate)
	return float64(dr)/dt <= c && float64(dtx)/dt <= c
}
