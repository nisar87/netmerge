package engine

// The Windows VPN capture (test) on a real Windows machine, with traffic: the engine takes packets from NetMerge's
// VPN plug-in (the "NetMerge Windows VPN (test)" helper, installed before this test runs) and sends them out on the
// machine's line. Measures download speed through the Windows VPN and without it, on the same machine, so the cost
// of the Windows VPN path is a number. Only the download test's addresses are routed into the VPN, so the machine's
// own connection to GitHub is never touched.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A file on GitHub's own servers, which GitHub's machines reach at full speed (two public speed-test servers
// refused them).
const speedURL = "https://github.com/nisar87/netmerge/releases/download/winvpn-test/NetMergeVpnTest.zip"

func TestWinVPNCaptureSpeed(t *testing.T) {
	if os.Getenv("NM_WINVPN_ENGINE_E2E") == "" {
		t.Skip("set NM_WINVPN_ENGINE_E2E=1 on a Windows machine with the helper installed")
	}
	pfn := strings.TrimSpace(psOut(t, "(Get-AppxPackage -Name NetMerge.WindowsVpnTest).PackageFamilyName"))
	if pfn == "" {
		t.Fatal("the helper is not installed")
	}
	ifName, _, _ := defaultRoute(0)
	if ifName == "" {
		t.Fatal("no default route")
	}
	// where the download really comes from (github.com redirects to its file servers)
	final := speedURL
	if resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(speedURL); err == nil {
		final = resp.Request.URL.String()
		resp.Body.Close()
	}
	u, _ := url.Parse(final)
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", u.Hostname())
	if err != nil || len(ips) == 0 {
		t.Fatalf("resolve %s: %v", u.Hostname(), err)
	}
	// one range covering the file servers (they share a /16), so the link fits in a 261-character task command
	v4 := ips[0].To4()
	routes := []string{fmt.Sprintf("%d.%d.0.0/16", v4[0], v4[1])}
	t.Logf("line %s; file server %s; routing %s into the VPN", ifName, u.Hostname(), strings.Join(routes, ","))

	direct1 := measure(t, "direct, 1 download", 1, final)
	direct4 := measure(t, "direct, 4 downloads", 4, final)
	results := []string{fmt.Sprintf("direct %.0f/%.0f", direct1, direct4)}
	for _, mtu := range []int{1500, 9000} {
		v1, v4 := vpnRun(t, ifName, routes, mtu, final)
		results = append(results, fmt.Sprintf("Windows VPN MTU %d %.0f/%.0f", mtu, v1, v4))
	}
	t.Logf("RESULT Mbps (1 download/4 downloads): %s", strings.Join(results, "   "))
}

// vpnRun starts the engine on the Windows VPN with the given MTU, connects the helper, measures, and stops.
func vpnRun(t *testing.T, ifName string, routes []string, mtu int, src string) (float64, float64) {
	probe, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	token := "0123456789abcdef" // short: the whole link has to fit in a 261-character task command
	dir := t.TempDir()
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "find-process-mode": "always",
		"external-controller": "127.0.0.1:0", "secret": "s",
		"tun": map[string]any{"enable": true, "auto-route": true, "auto-detect-interface": true, "mtu": mtu,
			"device": "winvpn", "vpn-port": port, "vpn-token": token},
		"proxies":      []map[string]any{{"name": "LINK1", "type": "direct", "interface-name": ifName, "udp": true}},
		"proxy-groups": []map[string]any{{"name": "AUTO", "type": "load-balance", "strategy": "round-robin", "proxies": []string{"LINK1"}}},
		"rules":        []string{"MATCH,AUTO"},
		"dns":          map[string]any{"enable": false},
	}
	b, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, b, 0o644)
	logs := &wbuf{}
	e := New(dir, logs)
	if err := e.Start(path); err != nil {
		t.Fatalf("start: %v\n%s", err, logs)
	}
	defer func() {
		openHelperLink(t, "netmerge-vpn:disconnect")
		e.Stop()
		t.Logf("MTU %d engine log (last part):\n%s", mtu, tail(logs.String(), 1500))
	}()
	plugin := fmt.Sprintf("<nm><port>%d</port><token>%s</token><mtu>%d</mtu><routes>%s</routes></nm>", port, token, mtu, strings.Join(routes, ","))
	began := time.Now()
	openHelperLink(t, "netmerge-vpn:connect?cfg="+base64.StdEncoding.EncodeToString([]byte(plugin)))
	for !strings.Contains(logs.String(), "capturing traffic through the Windows VPN") {
		if time.Since(began) > 45*time.Second {
			t.Errorf("MTU %d: the VPN did not come up in 45 s", mtu)
			return 0, 0
		}
		time.Sleep(200 * time.Millisecond)
	}
	if out := psOut(t, "Get-NetIPInterface -InterfaceAlias NetMerge -AddressFamily IPv4 | Format-List NlMtu"); out != "" {
		t.Logf("MTU %d: Windows gave the VPN %s", mtu, strings.TrimSpace(out))
	}
	time.Sleep(2 * time.Second)
	v1 := measure(t, fmt.Sprintf("Windows VPN MTU %d, 1 download", mtu), 1, src)
	v4 := measure(t, fmt.Sprintf("Windows VPN MTU %d, 4 downloads", mtu), 4, src)
	if !strings.Contains(logs.String(), "198.18.0.1:") {
		t.Errorf("MTU %d: the downloads did not go through the engine", mtu)
	}
	return v1, v4
}

// measure downloads from Cloudflare's speed test for 8 seconds with n parallel downloads and returns Mbps.
func measure(t *testing.T, what string, n int, src string) float64 {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var total atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
			for ctx.Err() == nil {
				req, _ := http.NewRequestWithContext(ctx, "GET", src, nil)
				resp, err := c.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						t.Logf("%s: %v", what, err)
					}
					return
				}
				if resp.StatusCode != 200 {
					t.Logf("%s: HTTP %d", what, resp.StatusCode)
					resp.Body.Close()
					return
				}
				n, _ := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				total.Add(n)
			}
		}()
	}
	wg.Wait()
	mbps := float64(total.Load()) * 8 / time.Since(start).Seconds() / 1e6
	t.Logf("%s: %.0f Mbps (%d MB)", what, mbps, total.Load()>>20)
	return mbps
}

// openHelperLink opens the helper's link as the signed-in user without administrator rights (as NetMerge does).
func openHelperLink(t *testing.T, link string) {
	const task = "NetMerge VPN helper link (test)"
	cmd := "powershell.exe -NoProfile -WindowStyle Hidden -Command Start-Process '" + link + "'"
	if len(cmd) > 261 {
		t.Fatalf("task command too long (%d)", len(cmd))
	}
	if out, err := exec.Command("schtasks.exe", "/Create", "/F", "/TN", task, "/SC", "ONCE", "/ST", "00:00", "/RL", "LIMITED", "/IT", "/TR", cmd).CombinedOutput(); err != nil {
		t.Logf("schtasks create: %v %s", err, out)
		return
	}
	if out, err := exec.Command("schtasks.exe", "/Run", "/TN", task).CombinedOutput(); err != nil {
		t.Logf("schtasks run: %v %s", err, out)
	}
	time.Sleep(3 * time.Second)
	exec.Command("schtasks.exe", "/Delete", "/F", "/TN", task).Run()
}

func psOut(t *testing.T, script string) string {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Logf("powershell: %v %s", err, out)
	}
	return string(out)
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
