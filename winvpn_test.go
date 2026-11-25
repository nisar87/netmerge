package main

import (
	"archive/zip"
	"encoding/base64"
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
	b, err := base64.URLEncoding.DecodeString(strings.TrimPrefix(link, "netmerge-vpn:connect?cfg="))
	if err != nil || string(b) != cfg || strings.ContainsAny(strings.TrimPrefix(link, "netmerge-vpn:connect?cfg="), "+/") {
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
	if s.VPNPort < 20000 || s.VPNPort > 60000 || len(s.VPNToken) < 32 {
		t.Fatalf("port %d token %q", s.VPNPort, s.VPNToken)
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
