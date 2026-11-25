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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const speedHost = "speed.cloudflare.com"

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
	ips, err := net.DefaultResolver.LookupIP(context.Background(), "ip4", speedHost)
	if err != nil || len(ips) == 0 {
		t.Fatalf("resolve %s: %v", speedHost, err)
	}
	var routes []string
	for _, ip := range ips {
		routes = append(routes, ip.String()+"/32")
	}
	t.Logf("line %s; routing %s into the VPN", ifName, strings.Join(routes, ","))

	// direct first, before anything is set up
	direct1 := measure(t, "direct, 1 download", 1)
	direct4 := measure(t, "direct, 4 downloads", 4)

	probe, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	token := "0123456789abcdef0123456789abcdef"
	dir := t.TempDir()
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "find-process-mode": "always",
		"external-controller": "127.0.0.1:0", "secret": "s",
		"tun": map[string]any{"enable": true, "auto-route": true, "auto-detect-interface": true,
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
		e.Stop()
		t.Logf("engine log (last part):\n%s", tail(logs.String(), 4000))
	}()

	plugin := fmt.Sprintf("<nm><port>%d</port><token>%s</token><mtu>1500</mtu><routes>%s</routes></nm>", port, token, strings.Join(routes, ","))
	began := time.Now()
	openHelperLink(t, "netmerge-vpn:connect?cfg="+base64.StdEncoding.EncodeToString([]byte(plugin)))
	defer openHelperLink(t, "netmerge-vpn:disconnect")
	for !strings.Contains(logs.String(), "capturing traffic through the Windows VPN") {
		if time.Since(began) > 60*time.Second {
			t.Fatalf("the VPN did not come up in 60 s\n%s", logs)
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Logf("VPN up after %d ms", time.Since(began).Milliseconds())
	time.Sleep(2 * time.Second)

	vpn1 := measure(t, "through the Windows VPN, 1 download", 1)
	vpn4 := measure(t, "through the Windows VPN, 4 downloads", 4)
	if !strings.Contains(logs.String(), speedHost) && !strings.Contains(logs.String(), ips[0].String()) {
		t.Errorf("the downloads did not go through the engine (no %s in its log)", speedHost)
	}
	t.Logf("RESULT  direct: %.0f Mbps (1) / %.0f Mbps (4)   Windows VPN: %.0f Mbps (1) / %.0f Mbps (4)", direct1, direct4, vpn1, vpn4)
	if vpn1 <= 0 {
		t.Fatal("nothing came through the Windows VPN")
	}
}

// measure downloads from Cloudflare's speed test for 8 seconds with n parallel downloads and returns Mbps.
func measure(t *testing.T, what string, n int) float64 {
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
				req, _ := http.NewRequestWithContext(ctx, "GET", "https://"+speedHost+"/__down?bytes=200000000", nil)
				resp, err := c.Do(req)
				if err != nil {
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
