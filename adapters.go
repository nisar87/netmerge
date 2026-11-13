package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Adapter struct {
	Name     string `json:"name"`
	Index    int    `json:"index"`
	Desc     string `json:"desc"`
	IP       string `json:"ip"`
	Gateway  string `json:"gateway"`
	Kind     string `json:"kind"` // ethernet | wifi | mobile
	LinkRate string `json:"linkRate"`
	Up       bool   `json:"up"`
}

var (
	adaptersMu   sync.Mutex
	adapterCache []Adapter
	adapterAt    time.Time
)

var virtualHints = []string{"virtual", "hyper-v", "vmware", "virtualbox", "vpn", "sonicwall", "tap-", "wintun",
	"wireguard", "meta", "netmerge", "loopback", "pseudo", "fortinet", "cisco anyconnect", "zerotier", "tailscale", "npcap", "teredo"}

func isVirtual(name, desc string) bool {
	s := strings.ToLower(name + " " + desc)
	if strings.HasPrefix(strings.ToLower(name), "vethernet") {
		return true
	}
	for _, h := range virtualHints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

func kindOf(name, desc string) string {
	s := strings.ToLower(name + " " + desc)
	switch {
	case strings.Contains(s, "remote ndis") || strings.Contains(s, "mobile") || strings.Contains(s, "cellular") || strings.Contains(s, "android") || strings.Contains(s, "iphone"):
		return "mobile"
	case strings.Contains(s, "wi-fi") || strings.Contains(s, "wireless") || strings.Contains(s, "wlan") || strings.Contains(s, "802.11"):
		return "wifi"
	}
	return "ethernet"
}

// allowVirtual: NM_ALLOW_VIRTUAL=1 or the flag -allow-virtual (the flag also survives the restart as administrator) to treat virtual adapters as real connections. Meant for testing inside a
// virtual machine such as Windows Sandbox, whose only adapter is a Hyper-V one; it is off for everyone else.
var allowVirtual = os.Getenv("NM_ALLOW_VIRTUAL") != ""

var adapterRefreshing bool

// getAdapters returns physical adapters that have an internet gateway. The scan takes seconds on Windows,
// so unless force is set it never blocks: stale data is returned and refreshed in the background.
func getAdapters(force bool) []Adapter {
	adaptersMu.Lock()
	cache, fresh := adapterCache, time.Since(adapterAt) < 10*time.Second
	if !force && cache != nil {
		if !fresh && !adapterRefreshing {
			adapterRefreshing = true
			go refreshAdapters()
		}
		adaptersMu.Unlock()
		return cache
	}
	adaptersMu.Unlock()
	return refreshAdapters()
}

func refreshAdapters() []Adapter {
	list, err := listAdapters()
	adaptersMu.Lock()
	defer adaptersMu.Unlock()
	adapterRefreshing = false
	if err != nil {
		logf("adapter scan failed: %v", err)
		return adapterCache
	}
	var out []Adapter
	for _, a := range list {
		if (isVirtual(a.Name, a.Desc) && !allowVirtual) || a.IP == "" {
			continue
		}
		a.Kind = kindOf(a.Name, a.Desc)
		out = append(out, a)
	}
	logAdapterChanges(adapterCache, out, adapterAt.IsZero())
	adapterCache, adapterAt = out, time.Now()
	return out
}

// logAdapterChanges writes when a connection appears, goes away, or moves to another network (gateway).
func logAdapterChanges(before, now []Adapter, first bool) {
	desc := func(a Adapter) string {
		return fmt.Sprintf("%s (%s, %s, gateway %s, %s)", a.Name, a.Kind, a.Desc, maskIP(a.Gateway), a.LinkRate)
	}
	old := map[string]Adapter{}
	for _, a := range before {
		old[a.Name] = a
	}
	if first {
		var all []string
		for _, a := range now {
			all = append(all, desc(a))
		}
		logf("connections found: %d: %s", len(now), strings.Join(all, "; "))
		return
	}
	for _, a := range now {
		o, ok := old[a.Name]
		switch {
		case !ok:
			logf("connection appeared: %s", desc(a))
		case o.Gateway != a.Gateway:
			logf("connection changed network: %s (was gateway %s)", desc(a), maskIP(o.Gateway))
		}
		delete(old, a.Name)
	}
	for _, a := range old {
		logf("connection went away: %s", a.Name)
	}
}

func adapterByName(name string) (Adapter, bool) {
	for _, a := range getAdapters(false) {
		if a.Name == name {
			return a, true
		}
	}
	return Adapter{}, false
}
