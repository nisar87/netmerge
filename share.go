package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
)

// Proxy sharing: other devices on your network (a phone, a tablet) can send their traffic through NetMerge by
// using this PC as their Wi-Fi proxy. Their connections are then dealt out over all your lines like any other
// program's, and the ad blocking applies to them too.
//
// Safety: the proxy needs a user name and password, only accepts addresses from private networks, and the
// firewall rule opens the port to the local network only.

const shareCipher = "aes-128-gcm"

type ShareSettings struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`   // HTTP/SOCKS proxy (manual Wi-Fi proxy setting)
	SSPort  int    `json:"ssPort"` // Shadowsocks listener (apps such as Hiddify import it from a QR code)
	User    string `json:"user"`
	Pass    string `json:"pass"`
}

func shareCfg(s Settings) ShareSettings {
	c := ShareSettings{Port: 7890, User: "netmerge"}
	if s.Share != nil {
		c = *s.Share
	}
	if c.Port < 1024 || c.Port > 65535 {
		c.Port = 7890
	}
	if c.SSPort < 1024 || c.SSPort > 65535 || c.SSPort == c.Port {
		c.SSPort = 8388
	}
	if c.User == "" {
		c.User = "netmerge"
	}
	return c
}

func shareConfig(cfg map[string]any, s Settings) {
	c := shareCfg(s)
	if !c.Enabled || c.Pass == "" {
		return
	}
	cfg["allow-lan"] = true
	cfg["bind-address"] = "*"
	cfg["mixed-port"] = c.Port
	cfg["authentication"] = []string{c.User + ":" + c.Pass}
	cfg["listeners"] = []map[string]any{{
		"name": "nm-share", "type": "shadowsocks", "port": c.SSPort, "listen": "0.0.0.0",
		"password": c.Pass, "cipher": shareCipher, "udp": true,
	}}
	cfg["lan-allowed-ips"] = []string{"192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "127.0.0.1/32"}
}

type shareAddr struct {
	Iface string `json:"iface"`
	IP    string `json:"ip"`
}

// shareAddresses lists this PC's private addresses a phone could reach.
func shareAddresses() []shareAddr {
	var out []shareAddr
	ifs, _ := net.Interfaces()
	for _, in := range ifs {
		if in.Flags&net.FlagUp == 0 || in.Flags&net.FlagLoopback != 0 {
			continue
		}
		low := strings.ToLower(in.Name)
		if strings.Contains(low, "meta") || strings.Contains(low, "vmware") || strings.Contains(low, "vethernet") || strings.Contains(low, "loopback") {
			continue
		}
		addrs, _ := in.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip[0] == 169 || ip[0] == 198 && ip[1] == 18 || !ip.IsPrivate() {
				continue
			}
			out = append(out, shareAddr{in.Name, ip.String()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IP < out[j].IP })
	return out
}

func registerShareRoutes(mux *http.ServeMux) {
	state := func() map[string]any {
		c := shareCfg(getSettings())
		running, _, _, _ := engine.Status()
		return map[string]any{"enabled": c.Enabled, "port": c.Port, "ssPort": c.SSPort, "cipher": shareCipher, "user": c.User, "pass": c.Pass,
			"addresses": shareAddresses(), "running": running, "publicNets": publicNetworks(), "devices": deviceViews(getSnapshot().Devices, getSettings().DeviceNames)}
	}
	mux.HandleFunc("/api/share", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var in struct {
				Enabled *bool `json:"enabled"`
				NewPass bool  `json:"newPass"`
				Name    *struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in.Name != nil && in.Name.ID != "" && len(in.Name.ID) <= 64 {
				updateSettings(func(s *Settings) {
					if s.DeviceNames == nil {
						s.DeviceNames = map[string]string{}
					}
					s.DeviceNames[in.Name.ID] = in.Name.Name
				})
			}
			updateSettings(func(s *Settings) {
				c := shareCfg(*s)
				if in.Enabled != nil {
					c.Enabled = *in.Enabled
				}
				if c.Pass == "" || in.NewPass {
					c.Pass = randomHex(5)
				}
				s.Share = &c
			})
			if err := engine.Apply(); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			logf("proxy sharing: %s", map[bool]string{true: "on", false: "off"}[shareCfg(getSettings()).Enabled])
		}
		writeJSON(w, state())
	}))
}

var shareFwKey string

// syncShareFirewall makes the firewall rule match the proxy-sharing setting (only when it changed).
func syncShareFirewall() {
	c := shareCfg(getSettings())
	on := c.Enabled && c.Pass != ""
	key := fmt.Sprintf("%v:%d:%d", on, c.Port, c.SSPort)
	if key == shareFwKey {
		return
	}
	shareFwKey = key
	setShareFirewall(on, c.Port, c.SSPort)
}
