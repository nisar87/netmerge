package engine

// End-to-end test of capturing traffic: a small "internet" in a separate network namespace (a web server, a UDP
// echo and a DNS server behind a virtual cable), the engine with its virtual adapter in front, and ordinary
// programs (this test) talking to it. Needs root and the ip tool; skipped otherwise. CI runs it with sudo.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *safeBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func run(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
		t.Fatalf("%s: %v %s", strings.Join(args, " "), err, out)
	}
}

const farPy = `
import socket, struct, threading, http.server, socketserver, os
os.chdir(%q)
def udp():
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(("203.0.113.10",5353))
    while True:
        d,a=s.recvfrom(65535); s.sendto(b"echo:"+d,a)
def dns():
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(("10.200.0.2",53))
    while True:
        q,a=s.recvfrom(512); i=12; labels=[]
        while q[i]: labels.append(q[i+1:i+1+q[i]].decode()); i+=1+q[i]
        name=".".join(labels); qt=struct.unpack(">H",q[i+1:i+3])[0]; qe=i+5; hdr=q[:2]+b"\x81\x80"+q[4:6]
        if name=="far.test" and qt==1:
            s.sendto(hdr+b"\x00\x01\x00\x00\x00\x00"+q[12:qe]+b"\xc0\x0c"+struct.pack(">HHIH",1,1,60,4)+socket.inet_aton("203.0.113.10"),a)
        else:
            s.sendto(hdr+b"\x00\x00\x00\x00\x00\x00"+q[12:qe],a)
threading.Thread(target=udp,daemon=True).start()
threading.Thread(target=dns,daemon=True).start()
class S(socketserver.ThreadingTCPServer):  # not HTTPServer: it looks its own name up in DNS, which hangs here
    daemon_threads=True; allow_reuse_address=True
S(("203.0.113.10",8080), http.server.SimpleHTTPRequestHandler).serve_forever()
`

func TestTunEndToEnd(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	if _, err := exec.LookPath("ip"); err != nil {
		t.Skip("needs the ip tool")
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		t.Skip("needs /dev/net/tun")
	}
	// the far side
	exec.Command("ip", "netns", "del", "nmfar").Run()
	exec.Command("ip", "link", "del", "nmveth-h").Run()
	run(t, "ip", "netns", "add", "nmfar")
	t.Cleanup(func() { exec.Command("ip", "netns", "del", "nmfar").Run() })
	run(t, "ip", "link", "add", "nmveth-h", "type", "veth", "peer", "name", "nmveth-f")
	t.Cleanup(func() { exec.Command("ip", "link", "del", "nmveth-h").Run() })
	run(t, "ip", "link", "set", "nmveth-f", "netns", "nmfar")
	run(t, "ip", "addr", "add", "10.200.0.1/24", "dev", "nmveth-h")
	run(t, "ip", "link", "set", "nmveth-h", "up")
	for _, c := range [][]string{
		{"ip", "addr", "add", "10.200.0.2/24", "dev", "nmveth-f"}, {"ip", "link", "set", "nmveth-f", "up"},
		{"ip", "link", "set", "lo", "up"}, {"ip", "addr", "add", "203.0.113.10/32", "dev", "lo"},
		{"ip", "route", "add", "default", "via", "10.200.0.1"},
	} {
		run(t, append([]string{"ip", "netns", "exec", "nmfar"}, c...)...)
	}
	// the line (LINK1) reaches the far side through the cable
	run(t, "ip", "route", "add", "203.0.113.0/24", "via", "10.200.0.2", "dev", "nmveth-h", "metric", "100")
	www := t.TempDir()
	os.WriteFile(filepath.Join(www, "index.html"), []byte("hello from the far side"), 0o644)
	big := bytes.Repeat([]byte("0123456789abcdef"), 2<<20) // 32 MiB
	os.WriteFile(filepath.Join(www, "big.bin"), big, 0o644)
	srv := exec.Command("ip", "netns", "exec", "nmfar", "python3", "-c", fmt.Sprintf(farPy, www))
	srvOut := &safeBuf{}
	srv.Stdout, srv.Stderr = srvOut, srvOut
	if err := srv.Start(); err != nil {
		t.Skip("needs python3:", err)
	}
	t.Cleanup(func() { srv.Process.Kill(); srv.Wait() })

	// the engine: LINK2 is a line that cannot reach anything (loopback), first in the slot on purpose
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "filters"), 0o755)
	os.WriteFile(filepath.Join(dir, "filters", "nm_block.txt"), []byte("+.blocked.test\n+.allowed.test\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "filters", "nm_allow.txt"), []byte("+.allowed.test\n"), 0o644)
	cfg := map[string]any{
		"mode": "rule", "log-level": "info", "find-process-mode": "always",
		"external-controller": "127.0.0.1:0", "secret": "s",
		"tun": map[string]any{"enable": true, "auto-route": true, "route-address": []string{"203.0.113.0/24"}, "dns-hijack": []string{"any:53"}, "mtu": 9000},
		"proxies": []map[string]any{
			{"name": "LINK1", "type": "direct", "interface-name": "nmveth-h", "udp": true},
			{"name": "LINK2", "type": "direct", "interface-name": "lo", "udp": true},
		},
		"proxy-groups": []map[string]any{
			{"name": "SLOT1", "type": "select", "proxies": []string{"LINK2", "LINK1"}},
			{"name": "AUTO", "type": "load-balance", "strategy": "round-robin", "proxies": []string{"SLOT1"}},
		},
		"rule-providers": map[string]any{
			"nm_block": map[string]any{"type": "file", "behavior": "domain", "format": "text", "path": "./filters/nm_block.txt"},
			"nm_allow": map[string]any{"type": "file", "behavior": "domain", "format": "text", "path": "./filters/nm_allow.txt"},
		},
		"rules": []string{"AND,((RULE-SET,nm_block),(NOT,((RULE-SET,nm_allow)))),REJECT", "MATCH,AUTO"},
		"sniffer": map[string]any{"enable": true, "parse-pure-ip": true, "sniff": map[string]any{
			"HTTP": map[string]any{"ports": []any{80, "8080-8880"}}, "TLS": map[string]any{"ports": []any{443}}, "QUIC": map[string]any{"ports": []any{443}}}},
		"dns": map[string]any{"enable": true, "enhanced-mode": "fake-ip", "fake-ip-range": "198.18.0.1/16", "nameserver": []string{"10.200.0.2"}},
	}
	b, _ := json.Marshal(cfg)
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, b, 0o644)
	logs := &safeBuf{}
	e := New(dir, logs)
	if err := e.Start(path); err != nil {
		t.Fatalf("start: %v\n%s", err, logs)
	}
	stopped := false
	t.Cleanup(func() {
		if !stopped {
			e.Stop()
		}
		if t.Failed() {
			t.Logf("engine log:\n%s", logs)
		}
	})
	// wait until the far side's web server answers (asked straight through the cable, not through the engine)
	ready := false
	for i := 0; i < 100 && !ready; i++ {
		d := &net.Dialer{Timeout: 200 * time.Millisecond}
		bindDialer(d, "nmveth-h", netip.Addr{})
		if c, err := d.Dial("tcp", "203.0.113.10:8080"); err == nil {
			c.Close()
			ready = true
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !ready {
		t.Fatalf("the test web server did not start: %s", srvOut)
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}

	t.Run("a captured connection reaches the website, via the working line", func(t *testing.T) {
		start := time.Now()
		resp, err := client.Get("http://203.0.113.10:8080/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "hello from the far side" {
			t.Fatalf("body %q", body)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("took %v: the broken line should be given up quickly", d)
		}
		if !strings.Contains(logs.String(), "using AUTO[LINK1]") || !strings.Contains(logs.String(), "(engine.test)") {
			t.Fatal("log should name the line and the program")
		}
		// now the broken line is known: the next connection must be quick
		start = time.Now()
		resp, err = client.Get("http://203.0.113.10:8080/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if d := time.Since(start); d > 500*time.Millisecond {
			t.Fatalf("second connection took %v", d)
		}
	})

	t.Run("downloads are fast through the capture", func(t *testing.T) {
		start := time.Now()
		resp, err := client.Get("http://203.0.113.10:8080/big.bin")
		if err != nil {
			t.Fatal(err)
		}
		n, _ := io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		secs := time.Since(start).Seconds()
		mbps := float64(n) * 8 / secs / 1e6
		t.Logf("downloaded %d bytes at %.0f Mbps", n, mbps)
		if n != int64(len(big)) {
			t.Fatalf("got %d of %d bytes", n, len(big))
		}
		if mbps < 300 {
			t.Fatalf("only %.0f Mbps through the capture", mbps)
		}
	})

	t.Run("websites on the block list are refused by name", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://203.0.113.10:8080/", nil)
		req.Host = "ads.blocked.test"
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			t.Fatal("a blocked website answered")
		}
		req2, _ := http.NewRequest("GET", "http://203.0.113.10:8080/", nil)
		req2.Host = "www.allowed.test"
		resp, err := client.Do(req2)
		if err != nil {
			t.Fatalf("an allowed website failed: %v", err)
		}
		resp.Body.Close()
		if !strings.Contains(logs.String(), "ads.blocked.test:8080 match AND(((RuleSet,nm_block)") {
			t.Fatal("no REJECT line in NetMerge's form for the blocked website")
		}
	})

	t.Run("UDP goes out and the answers come back", func(t *testing.T) {
		c, err := net.ListenUDP("udp4", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		dst := &net.UDPAddr{IP: net.ParseIP("203.0.113.10"), Port: 5353}
		for i := 0; i < 3; i++ {
			c.WriteToUDP([]byte(fmt.Sprintf("ping%d", i)), dst)
			buf := make([]byte, 100)
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				t.Fatalf("no answer: %v", err)
			}
			if string(buf[:n]) != fmt.Sprintf("echo:ping%d", i) || !from.IP.Equal(dst.IP) {
				t.Fatalf("answer %q from %v", buf[:n], from)
			}
		}
	})

	t.Run("the built-in DNS gives names, and connections by name work", func(t *testing.T) {
		c, err := net.Dial("udp", "198.18.0.2:53")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		c.Write(buildQuery("far.test", 1))
		buf := make([]byte, 512)
		n, err := c.Read(buf)
		if err != nil {
			t.Fatal(err)
		}
		as, _ := parseAnswers(buf[:n], 1)
		if len(as) != 1 || !tunNet4.Contains(as[0]) {
			t.Fatalf("expected a temporary address for far.test, got %v", as)
		}
		resp, err := client.Get("http://" + as[0].String() + ":8080/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(body) != "hello from the far side" {
			t.Fatalf("body %q", body)
		}
		if !strings.Contains(logs.String(), "--> far.test:8080 match") {
			t.Fatal("the log should show the website name")
		}
	})

	t.Run("stopping removes the adapter and its routes", func(t *testing.T) {
		e.Stop()
		stopped = true
		time.Sleep(200 * time.Millisecond)
		out, _ := exec.Command("ip", "link", "show", "nmtun").CombinedOutput()
		if !strings.Contains(string(out), "does not exist") {
			t.Fatalf("adapter still there: %s", out)
		}
		out, _ = exec.Command("ip", "route", "show", "203.0.113.0/24").Output()
		if strings.Contains(string(out), "nmtun") {
			t.Fatalf("route still there: %s", out)
		}
	})
}
