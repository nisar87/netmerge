//go:build windows

package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// TestWinvpnE2E runs on a real Windows machine (GitHub's runner) with the freshly built helper unpacked in
// NM_WINVPN_E2E_DIR: install it the way NetMerge does, ask it to connect the "NetMerge" Windows VPN, and check the
// plug-in reaches the engine's port on 127.0.0.1 with the right hello. Skipped elsewhere.
func TestWinvpnE2E(t *testing.T) {
	dir := os.Getenv("NM_WINVPN_E2E_DIR")
	if dir == "" {
		t.Skip("set NM_WINVPN_E2E_DIR to an unpacked NetMergeVpnTest.zip")
	}
	rootDir = t.TempDir()
	if err := winvpnInstall(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	pfn := winvpnInstalledPFN()
	t.Logf("installed: %s", pfn)

	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	s := Settings{VPNPort: l.LocalAddr().(*net.UDPAddr).Port, VPNToken: "0123456789abcdef0123456789abcdef"}
	hello := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, _, err := l.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if n > 0 && buf[0] == 0 {
				select {
				case hello <- string(buf[:n]):
				default:
				}
			}
		}
	}()

	res, err := winvpnRun("connect", winvpnPluginConfig(s, false))
	t.Logf("connect: %q %v", res, err)
	if out, e := powershell("Get-VpnConnection | Format-List Name,ConnectionStatus,TunnelType; Get-VpnConnection -AllUserConnection -ErrorAction SilentlyContinue | Format-List Name"); e == nil {
		t.Logf("Windows VPN profiles:\n%s", out)
	}
	select {
	case h := <-hello:
		if h != vpnHelloPrefixForTest+s.VPNToken {
			t.Fatalf("hello %q", h)
		}
		t.Logf("the plug-in reached the engine port with the right hello")
	case <-time.After(30 * time.Second):
		t.Errorf("no hello from the plug-in within 30 s")
	}
	if err != nil || res != "connected" {
		t.Fatalf("connect: %q %v", res, err)
	}
	if out, e := powershell("Get-NetIPAddress -IPAddress 198.18.0.1 -ErrorAction SilentlyContinue | Format-List InterfaceAlias,InterfaceIndex"); e == nil {
		t.Logf("VPN address:\n%s", strings.TrimSpace(string(out)))
	}
	res, err = winvpnRun("disconnect", "")
	t.Logf("disconnect: %q %v", res, err)
}

const vpnHelloPrefixForTest = "\x00NMVPN1 "
