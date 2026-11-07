package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const apiAddr = "127.0.0.1:9097"

// Known VPN clients: their tunnel traffic must stay on one link.
var vpnProcesses = []string{
	"swgvc.exe", "neservice.exe", "negui.exe", "mobileconnect.exe", "sonicwall mobile connect.exe",
	"vpnui.exe", "vpnagent.exe", "openvpn.exe", "openvpn-gui.exe", "wireguard.exe", "forticlient.exe",
	"fortisslvpndaemon.exe", "pangps.exe", "pangpa.exe", "nordvpn.exe", "expressvpn.exe", "anydesk.exe",
}

// procRule matches a program name case-insensitively (Windows reports e.g. OUTLOOK.EXE).
func procRule(name, target string) string {
	return "PROCESS-NAME-REGEX,(?i)^" + regexp.QuoteMeta(name) + "$," + target
}

func linkProxyName(i int) string { return fmt.Sprintf("LINK%d", i+1) }

func configDir() string  { return filepath.Join(rootDir, "config") }
func configPath() string { return filepath.Join(configDir(), "config.yaml") }

// activeLinks returns enabled links whose adapter is currently present, with their proxy base names.
type activeLink struct {
	Link
	Proxy string
	Kind  string
	Dead  bool // no working internet for a while: kept out of the groups
}

func activeLinks(s Settings) []activeLink {
	present := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		present[a.Name] = a
	}
	var out []activeLink
	for i, l := range s.Links {
		a, ok := present[l.Name]
		if !l.Enabled || !ok {
			continue
		}
		out = append(out, activeLink{Link: l, Proxy: linkProxyName(i), Kind: a.Kind, Dead: isDead(l.Name) || isQuotaOut(l.Name)})
	}
	// a backup line is only used while no other line works
	primary := false
	for _, l := range out {
		if l.Role != "backup" && !l.Dead {
			primary = true
		}
	}
	for i := range out {
		if out[i].Role == "backup" && primary {
			out[i].Dead = true
		}
	}
	return out
}

// proxyToLink maps a mihomo proxy name (e.g. "LINK2#3") back to the adapter name.
func proxyToLink(s Settings, proxy string) string {
	base := proxy
	if i := strings.Index(base, "#"); i > 0 {
		base = base[:i]
	}
	for i, l := range s.Links {
		if linkProxyName(i) == base {
			return l.Name
		}
	}
	return ""
}

func buildConfig(s Settings) ([]byte, error) {
	links := activeLinks(s)
	var proxies []map[string]any
	// links that answer are balanced over; if none answers, keep them all (better than no group at all)
	usable := map[string]bool{}
	for _, l := range links {
		if !l.Dead {
			usable[l.Name] = true
		}
	}
	if len(usable) == 0 {
		for _, l := range links {
			usable[l.Name] = true
		}
	}
	var weighted []string
	for _, l := range links {
		proxies = append(proxies, map[string]any{"name": l.Proxy, "type": "direct", "interface-name": l.Name, "udp": true})
		if usable[l.Name] {
			weighted = append(weighted, l.Proxy)
		}
		for w := 2; w <= l.Weight; w++ {
			alias := fmt.Sprintf("%s#%d", l.Proxy, w)
			proxies = append(proxies, map[string]any{"name": alias, "type": "direct", "interface-name": l.Name, "udp": true})
			if usable[l.Name] {
				weighted = append(weighted, alias)
			}
		}
	}

	groups := []map[string]any{}
	fast, stable := "DIRECT", "DIRECT"
	if len(links) > 0 {
		// No engine-side health checks: NetMerge probes each link itself (health.go) and rebuilds these lists.
		lb := func(name, strategy string, members []string) map[string]any {
			return map[string]any{"name": name, "type": "load-balance", "strategy": strategy, "proxies": members}
		}
		groups = append(groups, lb("FAST", "round-robin", weighted))
		groups = append(groups, lb("STABLE", "consistent-hashing", weighted))
		fast, stable = "FAST", "STABLE"

		// Smart dispatcher (dispatch.go): AUTO spreads every new connection over dispatchSlots small selectors
		// (instantly, one after the other - a burst of connections is split evenly); NetMerge decides which link each
		// selector points at, from the spare capacity of every link, with a tiny local API call when that changes.
		if !strings.HasPrefix(s.Mode, "link:") && !s.ClassicBalance {
			var members []string
			for _, l := range links {
				if usable[l.Name] {
					members = append(members, l.Proxy)
				}
			}
			if len(members) >= 2 {
				var slots []string
				for i := 1; i <= dispatchSlots; i++ {
					n := fmt.Sprintf("SLOT%d", i)
					slots = append(slots, n)
					groups = append(groups, map[string]any{"name": n, "type": "select", "proxies": members})
				}
				groups = append(groups, lb("AUTO", "round-robin", slots))
				fast = "AUTO"
			}
		}

		// "Only <connection>" mode: everything uses the chosen link; the others are used only while it is down.
		if strings.HasPrefix(s.Mode, "link:") {
			chosen := strings.TrimPrefix(s.Mode, "link:")
			if cp := proxyOf(links, chosen); cp != "" {
				members := []string{}
				if usable[chosen] {
					members = append(members, cp)
				}
				for _, l := range links {
					if l.Name != chosen && usable[l.Name] {
						members = append(members, l.Proxy)
					}
				}
				if len(members) == 0 {
					members = []string{cp}
				}
				groups = append(groups, map[string]any{"name": "ONLY", "type": "select", "proxies": members})
				fast, stable = "ONLY", "ONLY"
			}
		}
	}
	modeTarget := fast
	if s.Mode == "stable" {
		modeTarget = stable
	}

	// VPN link: chosen one if active, else first ethernet, else first.
	vpn := modeTarget
	if len(links) > 0 {
		vpn = links[0].Proxy
		for _, l := range links {
			if l.Kind == "ethernet" {
				vpn = l.Proxy
				break
			}
		}
		for _, l := range links {
			if l.Name == s.VPNLink {
				vpn = l.Proxy
			}
		}
	}

	target := func(t string) string {
		switch {
		case t == "fast":
			return fast
		case t == "stable":
			return stable
		case t == "bypass":
			return "DIRECT"
		case strings.HasPrefix(t, "link:"):
			name := strings.TrimPrefix(t, "link:")
			for _, l := range links {
				if l.Name == name {
					return l.Proxy
				}
			}
		}
		return modeTarget
	}

	var rules []string
	for _, ip := range s.BypassIPs {
		rules = append(rules, "IP-CIDR,"+ip+",DIRECT,no-resolve")
	}
	for _, p := range vpnProcesses {
		rules = append(rules, procRule(p, vpn))
	}
	rules = append(rules, "DST-PORT,500,"+vpn, "DST-PORT,4500,"+vpn)
	blocking := blockCfg(s).Enabled
	if blocking {
		// the allow list is checked first and just routes normally; everything else on the block list is refused
		rules = append(rules, "RULE-SET,nm_allow,"+modeTarget, "RULE-SET,nm_block,REJECT")
	}
	for _, r := range s.AppRules {
		rules = append(rules, procRule(r.Process, target(r.Target)))
	}
	for _, d := range s.StableDomains {
		rules = append(rules, "DOMAIN-SUFFIX,"+d+","+stable)
	}
	rules = append(rules, "DOMAIN-KEYWORD,bank,"+stable)
	for _, c := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "127.0.0.0/8"} {
		rules = append(rules, "IP-CIDR,"+c+",DIRECT,no-resolve")
	}
	rules = append(rules, "MATCH,"+modeTarget)

	exclude := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "224.0.0.0/4", "255.255.255.255/32"}
	exclude = append(exclude, s.BypassIPs...)

	stack := s.TunStack
	if stack != "system" && stack != "gvisor" {
		stack = "mixed"
	}
	tun := map[string]any{
		"enable": true, "stack": stack, "auto-route": true, "auto-detect-interface": true,
		"route-exclude-address": exclude,
	}
	if s.TunMTU >= 1280 && s.TunMTU <= 9000 {
		tun["mtu"] = s.TunMTU
	}
	cfg := map[string]any{
		"mode": "rule", "log-level": logLevel(s), "ipv6": false, "allow-lan": false,
		"find-process-mode": "always", "unified-delay": true, "tcp-concurrent": false,
		"external-controller": apiAddr, "secret": s.Secret,
		"profile":      map[string]any{"store-selected": false, "store-fake-ip": false},
		"tun":          tun,
		"proxies":      proxies,
		"proxy-groups": groups,
		"rules":        rules,
		"sniffer": map[string]any{
			"enable": true, "force-dns-mapping": true, "parse-pure-ip": true, "override-destination": false,
			"sniff": map[string]any{
				"HTTP": map[string]any{"ports": []any{80, "8080-8880"}},
				"TLS":  map[string]any{"ports": []any{443, 8443}},
				"QUIC": map[string]any{"ports": []any{443, 8443}},
			},
		},
	}
	shareConfig(cfg, s)
	if blocking {
		fileProv := func(f string) map[string]any {
			return map[string]any{"type": "file", "behavior": "domain", "format": "text", "path": "./filters/" + f}
		}
		cfg["rule-providers"] = map[string]any{"nm_block": fileProv("nm_block.txt"), "nm_allow": fileProv("nm_allow.txt")}
	}
	if s.BuiltinDNS {
		tun["dns-hijack"] = []string{"any:53"}
		cfg["dns"] = map[string]any{
			"enable": true, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16",
			"fake-ip-filter": []string{"+.lan", "+.local", "+.msftconnecttest.com", "+.msftncsi.com", "+.corp", "+.internal"},
			"nameserver":     []string{"1.1.1.1", "8.8.8.8"},
		}
	} else {
		// Keep Windows DNS untouched (works with VPN / company DNS); domains are learned by sniffing.
		cfg["dns"] = map[string]any{"enable": false}
	}
	// YAML is a superset of JSON, so mihomo reads this directly.
	return json.MarshalIndent(cfg, "", "  ")
}

func proxyOf(links []activeLink, name string) string {
	for _, l := range links {
		if l.Name == name {
			return l.Proxy
		}
	}
	return ""
}

func logLevel(s Settings) string {
	if s.DebugLog || blockCfg(s).Enabled { // blocked connections are only reported at the "info" level
		return "info"
	}
	return "warning"
}

func writeConfig() error {
	s := getSettings()
	if blockCfg(s).Enabled {
		if err := rebuildFilters(s); err != nil {
			logf("could not prepare the block lists (%v) - blocking is off until this is fixed", err)
			s.Block = &BlockSettings{}
		}
	}
	b, err := buildConfig(s)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(configDir(), 0o755)
	header := []byte("# Generated by NetMerge - edit settings in the app, not here.\n")
	return os.WriteFile(configPath(), append(header, b...), 0o644)
}
