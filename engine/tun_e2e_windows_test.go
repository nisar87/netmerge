package engine

// End-to-end test of capturing traffic on real Windows (Wintun): run by CI on a Windows machine with administrator
// rights (NM_WIN_TUN_E2E=1) after scripts/get-wintun.py put wintun.dll in place. Only a few public addresses are
// sent through the virtual adapter (route-address), so the rest of the machine keeps its normal internet.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type wbuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *wbuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *wbuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func TestTunWindows(t *testing.T) {
	if os.Getenv("NM_WIN_TUN_E2E") == "" {
		t.Skip("set NM_WIN_TUN_E2E=1 on a Windows machine with administrator rights")
	}
	ifName, gw, _ := defaultRoute(0)
	if ifName == "" {
		t.Fatal("no default route")
	}
	t.Logf("default line: %s via %s", ifName, gw)
	dir := t.TempDir()
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "find-process-mode": "always",
		"external-controller": "127.0.0.1:0", "secret": "s",
		"tun": map[string]any{"enable": true, "auto-route": true, "auto-detect-interface": true,
			// 1.1.1.1/1.0.0.1: Cloudflare's web and DNS; 162.159.200.0/24: time.cloudflare.com (NTP over UDP)
			"route-address": []string{"1.1.1.1/32", "1.0.0.1/32", "162.159.200.0/24"}, "dns-hijack": []string{"any:53"}, "mtu": 9000},
		"proxies":      []map[string]any{{"name": "LINK1", "type": "direct", "interface-name": ifName, "udp": true}},
		"proxy-groups": []map[string]any{{"name": "AUTO", "type": "load-balance", "strategy": "round-robin", "proxies": []string{"LINK1"}}},
		"rules":        []string{"DOMAIN-SUFFIX,blocked.example,REJECT", "MATCH,AUTO"},
		"sniffer": map[string]any{"enable": true, "parse-pure-ip": true, "sniff": map[string]any{
			"HTTP": map[string]any{"ports": []any{80}}, "TLS": map[string]any{"ports": []any{443}}}},
		"dns": map[string]any{"enable": true, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"8.8.8.8"}},
	}
	b, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, b, 0o644)
	logs := &wbuf{}
	e := New(dir, logs)
	if err := e.Start(path); err != nil {
		t.Fatalf("start: %v\n%s", err, logs)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			e.Stop()
		}
		t.Logf("engine log:\n%s", logs)
	})
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}

	t.Run("a captured connection goes out over the real line, named with the program", func(t *testing.T) {
		resp, err := client.Get("http://1.1.1.1/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		l := logs.String()
		if !strings.Contains(l, "1.1.1.1:80") || !strings.Contains(l, "using AUTO[LINK1]") || !strings.Contains(l, ".test.exe") {
			t.Fatal("the log should show the connection, its line and the program")
		}
	})

	var fake netip.Addr
	t.Run("DNS to a captured server answers with a fake address", func(t *testing.T) {
		c, err := net.DialTimeout("udp", "1.1.1.1:53", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write(buildQuery("speed.cloudflare.com", 1))
		r := make([]byte, 1500)
		n, err := c.Read(r)
		if err != nil {
			t.Fatal(err)
		}
		addrs, _ := parseAnswers(r[:n], 1)
		if len(addrs) == 0 || !netip.MustParsePrefix("198.18.0.0/16").Contains(addrs[0]) {
			t.Fatalf("answer %v", addrs)
		}
		fake = addrs[0]
	})

	t.Run("HTTPS to the fake address reaches the real site, fast", func(t *testing.T) {
		if !fake.IsValid() {
			t.Skip("no fake address")
		}
		tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{ServerName: "speed.cloudflare.com"},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, netip.AddrPortFrom(fake, 443).String())
			}}
		c := &http.Client{Timeout: 60 * time.Second, Transport: tr}
		start := time.Now()
		resp, err := c.Get("https://speed.cloudflare.com/__down?bytes=50000000")
		if err != nil {
			t.Fatal(err)
		}
		n, _ := io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		d := time.Since(start)
		t.Logf("downloaded %d bytes in %v (%.0f Mbit/s)", n, d, float64(n)*8/d.Seconds()/1e6)
		if n != 50000000 {
			t.Fatalf("got %d bytes (HTTP %d)", n, resp.StatusCode)
		}
		if !strings.Contains(logs.String(), "speed.cloudflare.com:443") {
			t.Fatal("the log should name the site")
		}
	})

	t.Run("UDP is relayed (time from time.cloudflare.com)", func(t *testing.T) {
		c, err := net.DialTimeout("udp", "162.159.200.1:123", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		req := make([]byte, 48)
		req[0] = 0x1b // NTP version 3, client
		var resp []byte
		for i := 0; i < 3 && resp == nil; i++ {
			c.SetDeadline(time.Now().Add(3 * time.Second))
			c.Write(req)
			r := make([]byte, 512)
			if n, err := c.Read(r); err == nil && n >= 48 {
				resp = r[:n]
			}
		}
		if resp == nil {
			t.Fatal("no NTP answer")
		}
		secs := binary.BigEndian.Uint32(resp[40:44])
		got := time.Unix(int64(secs)-2208988800, 0)
		if d := time.Since(got); d > time.Hour || d < -time.Hour {
			t.Fatalf("NTP time %v", got)
		}
	})

	t.Run("blocking by name", func(t *testing.T) {
		c, err := net.DialTimeout("udp", "1.1.1.1:53", 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write(buildQuery("ads.blocked.example", 1))
		r := make([]byte, 1500)
		n, _ := c.Read(r)
		c.Close()
		addrs, _ := parseAnswers(r[:n], 1)
		if len(addrs) == 0 {
			t.Fatal("no fake address for the blocked name")
		}
		conn, err := net.DialTimeout("tcp", netip.AddrPortFrom(addrs[0], 80).String(), 5*time.Second)
		if err == nil {
			conn.SetDeadline(time.Now().Add(5 * time.Second))
			conn.Write([]byte("GET / HTTP/1.1\r\nHost: ads.blocked.example\r\n\r\n"))
			n, _ := conn.Read(make([]byte, 64))
			conn.Close()
			if n > 0 {
				t.Fatal("a blocked site answered")
			}
		}
		if !strings.Contains(logs.String(), "ads.blocked.example:80") {
			t.Fatal("the block should be logged with the name")
		}
	})

	t.Run("stopping removes the adapter and leaves normal internet", func(t *testing.T) {
		e.Stop()
		stopped = true
		time.Sleep(time.Second)
		ifs, _ := net.Interfaces()
		for _, i := range ifs {
			if i.Name == "NetMerge" {
				t.Fatal("the virtual adapter is still there")
			}
		}
		resp, err := client.Get("http://1.1.1.1/")
		if err != nil {
			t.Fatal("no internet after stop:", err)
		}
		resp.Body.Close()
	})
}
