package main

import (
	"sync"
	"time"
)

// Two lines that show the same public address almost certainly leave through the same provider (for example
// two Wi-Fi networks from one router), so merging them cannot add speed. We look up each line's public
// address in the background and warn about it.

// providerKick asks for a lookup now (a connection changed network); otherwise we look every 5 minutes.
var providerKick = make(chan struct{}, 1)

func providerLoop() {
	select {
	case <-time.After(20 * time.Second):
	case <-providerKick:
	}
	for {
		refreshPublicIPs()
		select {
		case <-time.After(5 * time.Minute):
		case <-providerKick:
			time.Sleep(2 * time.Second) // let the new network settle (address, DNS)
		}
	}
}

// recheckProviders forgets the public address of connections that changed network, so the same-provider
// warning does not stand on an old reading, and looks them up again at once.
func recheckProviders(names []string) {
	statsMu.Lock()
	for _, n := range names {
		delete(publicIPs, n)
	}
	statsMu.Unlock()
	select {
	case providerKick <- struct{}{}:
	default:
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
	var wg sync.WaitGroup
	for _, l := range s.Links {
		a, ok := present[l.Name]
		if !l.Enabled || !ok {
			continue
		}
		wg.Add(1)
		go func(name string, a Adapter) {
			defer wg.Done()
			if ip := publicIP(&a); ip != "" {
				statsMu.Lock()
				publicIPs[name] = ip
				statsMu.Unlock()
			}
		}(l.Name, a)
	}
	wg.Wait()
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
