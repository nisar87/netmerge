package main

import "testing"

// A restarted adapter counter once added about 4.3 GB to a day: the page lets the real figure be put back.
func TestUsageSetCorrectsAWrongDay(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	updateSettings(func(s *Settings) {
		s.Links = []Link{{Name: "Ethernet 6", Label: "Phone", Enabled: true, Weight: 1}}
	})
	usageMu.Lock()
	usage = map[string]map[string][2]uint64{"2026-11-10": {"Ethernet 6": {4506581830, 4381394025}}}
	usageLoaded = true
	usageMu.Unlock()

	if err := usageSet("Ethernet 6", "2026-11-10", 212e6, 86e6); err != nil {
		t.Fatal(err)
	}
	if got := usage["2026-11-10"]["Ethernet 6"]; got != [2]uint64{212e6, 86e6} {
		t.Fatalf("usage after the correction = %v", got)
	}
	if err := usageSet("Ethernet 6", "2026-11-10", 0, 0); err != nil || len(usage["2026-11-10"]) != 0 {
		t.Fatalf("zero must remove the entry: %v %v", err, usage["2026-11-10"])
	}
	if usageSet("Nope", "2026-11-10", 1, 1) == nil {
		t.Error("an unknown connection must be refused")
	}
	if usageSet("Ethernet 6", "10/11/2026", 1, 1) == nil {
		t.Error("a bad date must be refused")
	}
}
