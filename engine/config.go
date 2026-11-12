package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config is the part of the engine settings file (config.yaml, written by NetMerge as JSON) that this engine reads.
// Keys NetMerge writes for mihomo that have no meaning here (tun stack, profile, unified-delay ...) are ignored.
type Config struct {
	Mode               string                    `json:"mode"`
	LogLevel           string                    `json:"log-level"`
	IPv6               bool                      `json:"ipv6"`
	AllowLAN           bool                      `json:"allow-lan"`
	BindAddress        string                    `json:"bind-address"`
	FindProcessMode    string                    `json:"find-process-mode"`
	ExternalController string                    `json:"external-controller"`
	Secret             string                    `json:"secret"`
	MixedPort          int                       `json:"mixed-port"`
	Authentication     []string                  `json:"authentication"`
	LanAllowedIPs      []string                  `json:"lan-allowed-ips"`
	Listeners          []ListenerConfig          `json:"listeners"`
	Tun                TunConfig                 `json:"tun"`
	Proxies            []ProxyConfig             `json:"proxies"`
	Groups             []GroupConfig             `json:"proxy-groups"`
	Rules              []string                  `json:"rules"`
	RuleProviders      map[string]ProviderConfig `json:"rule-providers"`
	Sniffer            SnifferConfig             `json:"sniffer"`
	DNS                DNSConfig                 `json:"dns"`
}

type ListenerConfig struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Port     int    `json:"port"`
	Listen   string `json:"listen"`
	Password string `json:"password"`
	Cipher   string `json:"cipher"`
	UDP      bool   `json:"udp"`
}

type TunConfig struct {
	Enable              bool     `json:"enable"`
	AutoRoute           bool     `json:"auto-route"`
	AutoDetectInterface bool     `json:"auto-detect-interface"`
	RouteAddress        []string `json:"route-address"` // what to capture (default: everything)
	RouteExcludeAddress []string `json:"route-exclude-address"`
	DNSHijack           []string `json:"dns-hijack"`
	MTU                 int      `json:"mtu"`
}

type ProxyConfig struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	InterfaceName string `json:"interface-name"`
	UDP           bool   `json:"udp"`
}

type GroupConfig struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`     // select | load-balance
	Strategy string   `json:"strategy"` // round-robin | consistent-hashing (load-balance)
	Proxies  []string `json:"proxies"`
}

type ProviderConfig struct {
	Type     string `json:"type"`     // file
	Behavior string `json:"behavior"` // domain
	Format   string `json:"format"`   // text
	Path     string `json:"path"`
}

type SnifferConfig struct {
	Enable              bool                     `json:"enable"`
	ParsePureIP         bool                     `json:"parse-pure-ip"`
	OverrideDestination bool                     `json:"override-destination"`
	Sniff               map[string]SniffProtocol `json:"sniff"`
}

type SniffProtocol struct {
	Ports []any `json:"ports"` // numbers and "from-to" ranges
}

type DNSConfig struct {
	Enable            bool                `json:"enable"`
	EnhancedMode      string              `json:"enhanced-mode"`
	FakeIPRange       string              `json:"fake-ip-range"`
	Nameserver        []string            `json:"nameserver"`
	DefaultNameserver []string            `json:"default-nameserver"`
	NameserverPolicy  map[string][]string `json:"nameserver-policy"`
	FakeIPFilter      []string            `json:"fake-ip-filter"`
}

// LoadConfig reads dir/config.yaml. NetMerge writes JSON (YAML's subset); that is the only form accepted.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(stripComments(b), &c); err != nil {
		return nil, fmt.Errorf("the settings file is not valid JSON: %w", err)
	}
	if c.Mode == "" {
		c.Mode = "rule"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	return &c, nil
}

// stripComments blanks whole-line "#" comments (NetMerge writes one at the top; YAML allows them, JSON does not).
// Line numbers stay the same, so a JSON error still points at the right place.
func stripComments(b []byte) []byte {
	lines := bytes.Split(b, []byte("\n"))
	for i, l := range lines {
		if t := bytes.TrimSpace(l); len(t) > 0 && t[0] == '#' {
			lines[i] = nil
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

// resolvePath turns a provider path ("./filters/x.txt") into a file under the config folder; paths may not leave it.
func resolvePath(dir, p string) (string, error) {
	full := filepath.Clean(filepath.Join(dir, p))
	if rel, err := filepath.Rel(dir, full); err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q is outside the settings folder", p)
	}
	return full, nil
}

// portSet is a set of ports from the sniffer's "ports" list: numbers and "from-to" strings.
type portSet [][2]uint16

func parsePorts(list []any) portSet {
	var ps portSet
	for _, v := range list {
		switch x := v.(type) {
		case float64:
			ps = append(ps, [2]uint16{uint16(x), uint16(x)})
		case string:
			var a, b int
			if n, _ := fmt.Sscanf(x, "%d-%d", &a, &b); n == 2 {
				ps = append(ps, [2]uint16{uint16(a), uint16(b)})
			} else if n, _ := fmt.Sscanf(x, "%d", &a); n == 1 {
				ps = append(ps, [2]uint16{uint16(a), uint16(a)})
			}
		}
	}
	return ps
}

func (ps portSet) has(p uint16) bool {
	for _, r := range ps {
		if p >= r[0] && p <= r[1] {
			return true
		}
	}
	return false
}
