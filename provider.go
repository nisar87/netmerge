package main

import (
	"time"
)

// Two lines that show the same public address almost certainly leave through the same provider (for example
// two Wi-Fi networks from one router), so merging them cannot add speed. We look up each line's public
// address in the background and warn about it.

func providerLoop() {
	time.Sleep(20 * time.Second)
	for {
		refreshPublicIPs()
		time.Sleep(5 * time.Minute)
	}
}

func refreshPublicIPs() {
	if running, _, _, _ := engine.Status(); !running {
		return
	}
	s := getSettings()
	present := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		present[a.Name] = a
	}
	for _, l := range s.Links {
		a, ok := present[l.Name]
		if !l.Enabled || !ok {
			continue
		}
		if ip := publicIP(&a); ip != "" {
			statsMu.Lock()
			publicIPs[l.Name] = ip
			statsMu.Unlock()
		}
	}
}

// sameProviderLines returns the labels of enabled lines that share one public address (empty if all differ).
func sameProviderLines(links []LinkStat) []string {
	by := map[string][]string{}
	for _, l := range links {
		if l.Present && l.Enabled && l.PublicIP != "" {
			by[l.PublicIP] = append(by[l.PublicIP], l.Label)
		}
	}
	var out []string
	for _, names := range by {
		if len(names) > 1 {
			out = append(out, names...)
		}
	}
	return out
}
