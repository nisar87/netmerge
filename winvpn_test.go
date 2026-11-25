package main

import (
	"archive/zip"
	"encoding/base64"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWinvpnPluginConfigAndLink(t *testing.T) {
	s := Settings{VPNPort: 41234, VPNToken: "0123456789abcdef0123456789abcdef"}
	cfg := winvpnPluginConfig(s, false)
	if cfg != "<nm><port>41234</port><token>0123456789abcdef0123456789abcdef</token><mtu>1500</mtu></nm>" {
		t.Fatalf("got %s", cfg)
	}
	if !strings.Contains(winvpnPluginConfig(s, true), "<dns>198.18.0.2</dns>") {
		t.Fatal("built-in DNS on: Windows must be told to ask the engine")
	}
	link := winvpnLink("connect", cfg)
	v := strings.TrimPrefix(link, "netmerge-vpn:connect?cfg=")
	b, err := base64.StdEncoding.DecodeString(v) // what the helper does (.NET Convert.FromBase64String)
	if err != nil || string(b) != cfg || strings.ContainsAny(v, "-_%") {
		t.Fatalf("link %s", link)
	}
	if winvpnLink("disconnect", "") != "netmerge-vpn:disconnect" {
		t.Fatal(winvpnLink("disconnect", ""))
	}
}

func TestWinvpnSettingsGetPortAndToken(t *testing.T) {
	rootDir = t.TempDir()
	settings = defaultSettings()
	s := updateSettings(func(s *Settings) { s.Capture = "winvpn" })
	if len(s.VPNToken) < 32 {
		t.Fatalf("token %q", s.VPNToken)
	}
	if err := writeConfig(); err != nil {
		t.Fatal(err)
	}
	s = getSettings()
	if s.VPNPort < 1024 || s.VPNPort > 65535 {
		t.Fatalf("writing the engine settings must choose a port: %d", s.VPNPort)
	}
	again := updateSettings(func(s *Settings) {})
	if again.VPNPort != s.VPNPort || again.VPNToken != s.VPNToken {
		t.Fatal("the port and token must stay the same (the VPN profile holds them)")
	}
	if off := updateSettings(func(s *Settings) { s.Capture = "something" }); off.Capture != "" {
		t.Fatalf("unknown capture kept: %q", off.Capture)
	}
	cfg, _ := buildConfig(getSettings())
	if strings.Contains(string(cfg), "winvpn") {
		t.Fatal("the test switched off must not reach the engine")
	}
	updateSettings(func(s *Settings) { s.Capture = "winvpn" })
	cfg, _ = buildConfig(getSettings())
	if !strings.Contains(string(cfg), `"device": "winvpn"`) || !strings.Contains(string(cfg), `"vpn-token"`) {
		t.Fatalf("engine settings: %s", cfg)
	}
	if strings.Contains(strings.Join(func() []string {
		var o []string
		for k, v := range reportSettings(getSettings()) {
			o = append(o, k+"="+toString(v))
		}
		return o
	}(), " "), getSettings().VPNToken) {
		t.Fatal("the token must never be in reports")
	}
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func TestUnzipRefusesPathsOutside(t *testing.T) {
	dir := t.TempDir()
	zp := filepath.Join(dir, "a.zip")
	f, _ := os.Create(zp)
	w := zip.NewWriter(f)
	x, _ := w.Create("ok/file.txt")
	x.Write([]byte("hi"))
	y, _ := w.Create("../evil.txt")
	y.Write([]byte("no"))
	w.Close()
	f.Close()
	err := unzipTo(zp, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("a name leading outside must be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.txt")); err == nil {
		t.Fatal("written outside")
	}
}

// A port Windows keeps for itself (or one in use) is replaced by one Windows hands out; a usable one is kept.
func TestUsableVPNPort(t *testing.T) {
	busy, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	taken := busy.LocalAddr().(*net.UDPAddr).Port
	p := usableVPNPort(taken)
	if p == taken || p <= 0 {
		t.Fatalf("a port that cannot be bound must be replaced: %d", p)
	}
	busy.Close()
	if usableVPNPort(p) != p {
		t.Fatal("a usable port must be kept (the VPN profile holds it)")
	}
	if usableVPNPort(0) <= 0 {
		t.Fatal("no port chosen")
	}
}

func TestWinvpnLinkFitsATaskCommand(t *testing.T) {
	s := Settings{VPNPort: 65535, VPNToken: strings.Repeat("f", 32)}
	link := winvpnLink("connect", winvpnPluginConfig(s, true))
	cmd := "powershell.exe -NoProfile -WindowStyle Hidden -Command Start-Process '" + link + "'"
	if len(cmd) > 261 {
		t.Fatalf("task command too long (%d): %s", len(cmd), cmd)
	}
	if strings.ContainsAny(link, "' \\"+`"`) {
		t.Fatalf("the link needs no quoting: %s", link)
	}
}
