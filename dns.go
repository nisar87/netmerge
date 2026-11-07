package main

import (
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// DNS servers. Default (and what every older version did): Windows keeps handling name lookups. Optionally NetMerge
// answers lookups itself with a provider you choose, plain or encrypted (DNS-over-HTTPS). Nothing changes unless
// you apply a choice, and NetMerge goes back to Windows DNS by itself if the chosen servers stop answering.

type dnsProv struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Desc  string   `json:"desc"`
	Plain []string `json:"plain"`
	DoH   []string `json:"doh"`
	Boot  []string `json:"-"` // plain addresses used to find the DoH host name
}

var dnsProviders = []dnsProv{
	{ID: "cloudflare", Name: "Cloudflare (1.1.1.1)", Desc: "Very fast, no filtering, does not keep logs that identify you.",
		Plain: []string{"1.1.1.1", "1.0.0.1"}, DoH: []string{"https://1.1.1.1/dns-query", "https://1.0.0.1/dns-query"}},
	{ID: "google", Name: "Google (8.8.8.8)", Desc: "Very fast and reliable, no filtering. Google logs lookups.",
		Plain: []string{"8.8.8.8", "8.8.4.4"}, DoH: []string{"https://8.8.8.8/dns-query", "https://8.8.4.4/dns-query"}},
	{ID: "quad9", Name: "Quad9 (9.9.9.9)", Desc: "Blocks websites known to spread malware and phishing. Privacy-focused.",
		Plain: []string{"9.9.9.9", "149.112.112.112"}, DoH: []string{"https://9.9.9.9/dns-query", "https://149.112.112.112/dns-query"}},
	{ID: "opendns", Name: "OpenDNS", Desc: "Reliable, with optional filtering if you set up an account. No filtering by default.",
		Plain: []string{"208.67.222.222", "208.67.220.220"}, DoH: []string{"https://doh.opendns.com/dns-query"}, Boot: []string{"208.67.222.222", "208.67.220.220"}},
	{ID: "adguard", Name: "AdGuard DNS", Desc: "Blocks ads and trackers for every app. Some sites may load differently.",
		Plain: []string{"94.140.14.14", "94.140.15.15"}, DoH: []string{"https://dns.adguard-dns.com/dns-query"}, Boot: []string{"94.140.14.14", "94.140.15.15"}},
}

func findProv(id string) (dnsProv, bool) {
	for _, p := range dnsProviders {
		if p.ID == id {
			return p, true
		}
	}
	return dnsProv{}, false
}

// dnsActive: NetMerge answers lookups with a provider chosen by the user (not the Windows default or the old built-in pair).
func dnsActive(s Settings) bool {
	return s.BuiltinDNS && (s.DNSProvider == "custom" || func() bool { _, ok := findProv(s.DNSProvider); return ok }())
}

// dnsServersFor returns the nameserver list for the engine and the plain addresses used to find a DoH host.
func dnsServersFor(s Settings) (ns, boot []string) {
	if s.DNSProvider == "custom" {
		return append([]string(nil), s.DNSCustom...), append([]string(nil), s.DNSCustom...)
	}
	p, ok := findProv(s.DNSProvider)
	if !ok {
		return nil, nil
	}
	if s.DNSSecure {
		ns = append(ns, p.DoH...)
	} else {
		ns = append(ns, p.Plain...)
	}
	if len(p.Boot) > 0 {
		boot = p.Boot
	} else {
		boot = p.Plain
	}
	return ns, boot
}

// buildDNSConfig is the engine's "dns" section when built-in DNS is on.
func buildDNSConfig(s Settings) map[string]any {
	filter := []string{"+.lan", "+.local", "+.msftconnecttest.com", "+.msftncsi.com", "+.corp", "+.internal"}
	d := map[string]any{
		"enable": true, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16",
		"nameserver": []string{"1.1.1.1", "8.8.8.8"}, // the long-standing default of the built-in DNS
	}
	if ns, boot := dnsServersFor(s); dnsActive(s) && len(ns) > 0 {
		d["nameserver"] = ns
		d["default-nameserver"] = boot
	}
	if dnsActive(s) && len(s.DNSDomains) > 0 {
		pol := map[string]any{}
		for _, dom := range s.DNSDomains {
			pol["+."+dom] = []string{"system"} // company names keep using the Windows DNS
			filter = append(filter, "+."+dom)
		}
		d["nameserver-policy"] = pol
	}
	d["fake-ip-filter"] = filter
	return d
}

var domRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,24}$`)

func cleanDomains(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "*.")))
		d = strings.TrimPrefix(d, ".")
		if domRe.MatchString(d) && !seen[d] && len(out) < 20 {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func cleanIPs(in []string) []string {
	var out []string
	for _, v := range in {
		ip := net.ParseIP(strings.TrimSpace(v))
		if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
			continue
		}
		out = append(out, ip.String())
		if len(out) == 2 {
			break
		}
	}
	return out
}

// ---- safety: leave the Windows DNS if the chosen servers do not answer

var (
	dnsGuardMu    sync.Mutex
	dnsGuardFails int
)

func dnsRevert(why string) {
	updateSettings(func(s *Settings) { s.BuiltinDNS, s.DNSProvider, s.DNSFallback = false, "", true })
	note("bad", "%s NetMerge went back to the Windows DNS.", why)
	logf("dns: %s - back to Windows DNS", why)
}

func dnsGuardLoop() {
	time.Sleep(45 * time.Second)
	t := time.NewTicker(60 * time.Second)
	for range t.C {
		safely("dns guard", dnsGuardTick)
	}
}

func dnsGuardTick() {
	s := getSettings()
	if running, _, _, _ := engine.Status(); !running || !dnsActive(s) {
		dnsGuardMu.Lock()
		dnsGuardFails = 0
		dnsGuardMu.Unlock()
		return
	}
	// only judge the servers while at least one line has working internet
	var lines []Adapter
	present := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		present[a.Name] = a
	}
	for _, l := range s.Links {
		if a, ok := present[l.Name]; ok && l.Enabled && !isDead(l.Name) && !isPortal(l.Name) {
			lines = append(lines, a)
		}
	}
	if len(lines) == 0 {
		return
	}
	ok := false
	for _, a := range lines {
		if dnsServersAnswer(a, s) {
			ok = true
			break
		}
	}
	dnsGuardMu.Lock()
	if ok {
		dnsGuardFails = 0
		dnsGuardMu.Unlock()
		return
	}
	dnsGuardFails++
	f := dnsGuardFails
	dnsGuardMu.Unlock()
	if f >= 3 {
		dnsGuardMu.Lock()
		dnsGuardFails = 0
		dnsGuardMu.Unlock()
		dnsRevert("The chosen DNS servers stopped answering.")
		_ = engine.Apply()
	}
}

// ---- API

type dnsView struct {
	Provider string   `json:"provider"` // "windows" | "builtin" | provider id | "custom"
	Secure   bool     `json:"secure"`
	Custom   []string `json:"custom"`
	Domains  []string `json:"domains"`
	Fallback bool     `json:"fallback"`
}

func dnsViewOf(s Settings) dnsView {
	v := dnsView{Provider: "windows", Secure: s.DNSSecure, Custom: s.DNSCustom, Domains: s.DNSDomains, Fallback: s.DNSFallback}
	if v.Custom == nil {
		v.Custom = []string{}
	}
	if v.Domains == nil {
		v.Domains = []string{}
	}
	switch {
	case dnsActive(s):
		v.Provider = s.DNSProvider
	case s.BuiltinDNS:
		v.Provider = "builtin"
	}
	return v
}

func registerDNSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dns", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in struct {
				Action   string   `json:"action"`
				Provider string   `json:"provider"`
				Secure   bool     `json:"secure"`
				Custom   []string `json:"custom"`
				Domains  []string `json:"domains"`
			}
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
				writeErr(w, 400, "bad request")
				return
			}
			switch in.Action {
			case "test":
				if !startDNSTest() {
					writeErr(w, 409, "A test is already running")
					return
				}
			case "reset":
				updateSettings(func(s *Settings) { s.BuiltinDNS, s.DNSProvider, s.DNSFallback = false, "", false })
				note("info", "DNS: Windows default is used again")
				if err := engine.Apply(); err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			case "set":
				custom := cleanIPs(in.Custom)
				switch {
				case in.Provider == "windows":
					updateSettings(func(s *Settings) { s.BuiltinDNS, s.DNSProvider, s.DNSFallback = false, "", false })
				case in.Provider == "custom":
					if len(custom) == 0 {
						writeErr(w, 400, "Enter at least one valid IPv4 address, for example 9.9.9.9")
						return
					}
				default:
					if _, ok := findProv(in.Provider); !ok {
						writeErr(w, 400, "Unknown DNS provider")
						return
					}
				}
				if in.Provider != "windows" {
					doms := cleanDomains(in.Domains)
					updateSettings(func(s *Settings) {
						s.BuiltinDNS, s.DNSProvider, s.DNSFallback = true, in.Provider, false
						s.DNSSecure = in.Secure && in.Provider != "custom"
						s.DNSCustom, s.DNSDomains = custom, doms
					})
					name := "your own servers"
					if p, ok := findProv(in.Provider); ok {
						name = p.Name
					}
					note("info", "DNS: now using %s%s", name, map[bool]string{true: " (encrypted)", false: ""}[in.Secure && in.Provider != "custom"])
				}
				if err := engine.Apply(); err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			default:
				writeErr(w, 400, "unknown action")
				return
			}
		}
		writeJSON(w, map[string]any{"dns": dnsViewOf(getSettings()), "providers": dnsProviders, "test": getDNSTest()})
	}))
}

// dnsStartFailed: if the engine would not start and its message points at the DNS settings, those settings are put
// back to the Windows default so that the next try works (merging must never be lost over a DNS choice).
func dnsStartFailed(msg string) {
	l := strings.ToLower(msg)
	if !dnsActive(getSettings()) || !(strings.Contains(l, "dns") || strings.Contains(l, "nameserver") || strings.Contains(l, "policy")) {
		return
	}
	dnsRevert("The network engine could not start with the chosen DNS settings.")
}
