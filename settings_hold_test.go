package main

import (
	"strings"
	"testing"
	"time"
)

// The sampler (and so the window) waits for the settings: a long hold is logged with where it happened and how
// much of it was saving the file, so a report shows whether a slow disk or slow code held the window up.
func TestLongSettingsHoldIsLogged(t *testing.T) {
	read := withTempLog(t)
	old := settingsSlow
	settingsSlow = 20 * time.Millisecond
	t.Cleanup(func() { settingsSlow = old })

	getSettings()
	updateSettings(func(s *Settings) {})
	if got := read(); strings.Contains(got, "settings were held") {
		t.Fatalf("a quick hold was logged: %q", got)
	}

	updateSettings(func(s *Settings) { time.Sleep(50 * time.Millisecond) })
	got := read()
	if !strings.Contains(got, "the settings were held for") || !strings.Contains(got, "settings_hold_test.go:") ||
		!strings.Contains(got, "saving the file") {
		t.Fatalf("a long hold was not logged with its place: %q", got)
	}
}
