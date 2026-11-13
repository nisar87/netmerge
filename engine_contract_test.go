package main

// The engine contract: everything NetMerge needs from the network engine, checked against a real engine binary.
// Run with NM_ENGINE_BIN=/path/to/engine go test -run TestEngineContract -v
// (NetMerge itself as the engine: NM_ENGINE_BIN=./netmerge NM_ENGINE_ARGS=-engine)
// CI runs it against the engine on its own (nmengine) and as NetMerge runs it (NetMerge -engine).
// The virtual adapter (TUN) needs Windows and admin rights, so here traffic enters through the sharing ports
// (HTTP/SOCKS proxy and Shadowsocks) - the rules, line choice, API and log are the same for both ways in.

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type contractEngine struct {
	t       *testing.T
	dir     string
	api     string
	secret  string
	proxy   string // HTTP/SOCKS port
	ss      string // Shadowsocks port
	user    string
	pass    string
	mu      sync.Mutex
	out     bytes.Buffer
	cmd     *exec.Cmd
	pumpEnd chan struct{}
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// secondInterface returns a real network interface other than loopback, so the config has two lines.
func secondInterface() string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagLoopback == 0 && i.Flags&net.FlagUp != 0 {
			return i.Name
		}
	}
	return ""
}

func startContractEngine(t *testing.T, bin string) *contractEngine {
	second := secondInterface()
	if second == "" {
		t.Skip("needs a network interface besides loopback")
	}
	rootDir = t.TempDir()
	settings = defaultSettings()
	// two lines: LINK1 is loopback (it can reach the test servers), LINK2 is the machine's real interface
	adapterCache, adapterAt = []Adapter{{Name: "lo", Kind: "ethernet", Up: true}, {Name: second, Kind: "wifi", Up: true}}, time.Now().Add(time.Hour)
	t.Cleanup(func() { adapterCache, adapterAt = nil, time.Time{} })
	s := getSettings()
	s.Links = []Link{{Name: "lo", Label: "Loop", Enabled: true, Weight: 1}, {Name: second, Label: "Real", Enabled: true, Weight: 1}}
	s.Block = defaultBlock()
	s.Block.Enabled = true
	e := &contractEngine{t: t, dir: configDir(), secret: "contract-secret", user: "netmerge", pass: "contract-pass"}
	s.Secret = e.secret
	pp, sp := freePort(t), freePort(t)
	s.Share = &ShareSettings{Enabled: true, Port: pp, SSPort: sp, User: e.user, Pass: e.pass}
	b, err := buildConfig(s)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	// test-only changes: no TUN (needs admin), a free API port, and the test server's address (127.0.0.1) is
	// taken out of the "local addresses go direct" rule so it can stand in for a website on the internet
	rules := []any{}
	for _, r := range cfg["rules"].([]any) {
		if r != "IP-CIDR,127.0.0.0/8,DIRECT,no-resolve" {
			rules = append(rules, r)
		}
	}
	if len(rules) != len(cfg["rules"].([]any))-1 {
		t.Fatal("the local-address rule NetMerge writes has changed; update the contract test")
	}
	cfg["rules"] = rules
	if cfg["log-level"] != "info" {
		t.Fatal("with blocking on NetMerge must ask for the info log level (blocked lines are only logged there)")
	}
	cfg["tun"].(map[string]any)["enable"] = false
	e.api = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg["external-controller"] = e.api
	e.proxy, e.ss = fmt.Sprintf("127.0.0.1:%d", pp), fmt.Sprintf("127.0.0.1:%d", sp)
	out, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.MkdirAll(filepath.Join(e.dir, "filters"), 0o755)
	// exactly as NetMerge writes it, header line included
	if err := os.WriteFile(filepath.Join(e.dir, "config.yaml"), append([]byte(configHeader), out...), 0o644); err != nil {
		t.Fatal(err)
	}
	// block list: blocked.test and allowed.test; allow list: allowed.test (so it must NOT be refused)
	_ = os.WriteFile(filepath.Join(e.dir, "filters", "nm_block.txt"), []byte("+.blocked.test\n+.allowed.test\n"), 0o644)
	_ = os.WriteFile(filepath.Join(e.dir, "filters", "nm_allow.txt"), []byte("+.allowed.test\n"), 0o644)

	var pre []string
	if v := os.Getenv("NM_ENGINE_ARGS"); v != "" { // e.g. "-engine" to run NetMerge.exe itself as the engine
		pre = strings.Fields(v)
	}
	e.cmd = exec.Command(bin, append(pre, "-d", e.dir)...)
	pr, pw, _ := os.Pipe()
	e.cmd.Stdout, e.cmd.Stderr = pw, pw
	if err := e.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pw.Close()
	// NetMerge's own log reader counts the blocks; a copy of every line is kept for the checks below
	cr, cw, _ := os.Pipe()
	e.pumpEnd = make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			e.mu.Lock()
			e.out.WriteString(sc.Text() + "\n")
			e.mu.Unlock()
			cw.WriteString(sc.Text() + "\n")
		}
		cw.Close()
	}()
	go func() { pumpEngineLog(cr, nil); close(e.pumpEnd) }()
	t.Cleanup(func() {
		_ = e.cmd.Process.Kill()
		_, _ = e.cmd.Process.Wait()
		<-e.pumpEnd
		if t.Failed() {
			t.Logf("engine output:\n%s", e.log())
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, code := e.req("GET", "/version", nil, true); code == 200 {
			return e
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("engine did not answer on %s:\n%s", e.api, e.log())
	return nil
}

func (e *contractEngine) log() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.out.String()
}

func (e *contractEngine) req(method, path string, body any, auth bool) ([]byte, int) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r, _ := http.NewRequest(method, "http://"+e.api+path, rd)
	if auth {
		r.Header.Set("Authorization", "Bearer "+e.secret)
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(r)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode
}

func (e *contractEngine) proxies() map[string]struct {
	Now  string   `json:"now"`
	All  []string `json:"all"`
	Type string   `json:"type"`
} {
	b, code := e.req("GET", "/proxies", nil, true)
	if code != 200 {
		e.t.Fatalf("/proxies answered %d", code)
	}
	var r struct {
		Proxies map[string]struct {
			Now  string   `json:"now"`
			All  []string `json:"all"`
			Type string   `json:"type"`
		} `json:"proxies"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		e.t.Fatalf("/proxies is not the expected JSON: %v", err)
	}
	return r.Proxies
}

func (e *contractEngine) connections() []mConn {
	b, code := e.req("GET", "/connections", nil, true)
	if code != 200 {
		e.t.Fatalf("/connections answered %d", code)
	}
	var l connList
	if err := json.Unmarshal(b, &l); err != nil {
		e.t.Fatalf("/connections is not the expected JSON: %v", err)
	}
	return l.Connections
}

// httpVia sends a request through the sharing proxy port.
func (e *contractEngine) httpVia(target string, withAuth bool) (*http.Response, error) {
	pu, _ := url.Parse("http://" + e.proxy)
	tr := &http.Transport{Proxy: http.ProxyURL(pu)}
	r, _ := http.NewRequest("GET", target, nil)
	if withAuth {
		r.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(e.user+":"+e.pass)))
	}
	return (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Do(r)
}

// connectVia opens a CONNECT tunnel to host:port through the sharing proxy. It reports whether the tunnel really
// carries data: an engine may answer "200" and close a refused tunnel at once, or answer an error.
func (e *contractEngine) connectVia(hostport string) (open bool) {
	c, err := net.DialTimeout("tcp", e.proxy, 5*time.Second)
	if err != nil {
		e.t.Fatal(err)
	}
	defer c.Close()
	auth := base64.StdEncoding.EncodeToString([]byte(e.user + ":" + e.pass))
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n\r\n", hostport, hostport, auth)
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if !strings.Contains(line, " 200") {
		return false
	}
	for { // rest of the answer's header
		l, err := br.ReadString('\n')
		if err != nil {
			return false
		}
		if strings.TrimSpace(l) == "" {
			break
		}
	}
	// a refused tunnel is closed straight away; an open one stays open (nothing is listening at the far end here,
	// so "open" means the engine is still trying to reach it rather than refusing)
	_ = c.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	_, err = br.ReadByte()
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	return false
}

// a local web server: /slow sends 64 KB pieces for about two seconds, so a connection stays open long enough to see
func contractServer(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64<<10)
		for i := 0; i < 20; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			time.Sleep(100 * time.Millisecond)
		}
	})
	mux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "hello from the contract server") })
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return l.Addr().String()
}

func TestEngineContract(t *testing.T) {
	bin := os.Getenv("NM_ENGINE_BIN")
	if bin == "" {
		t.Skip("set NM_ENGINE_BIN to an engine binary to run the engine contract")
	}
	e := startContractEngine(t, bin)
	srv := contractServer(t)
	_, port, _ := net.SplitHostPort(srv)
	local := "localhost:" + port // a name, so it goes through the rules to the merge (127.0.0.1 itself goes DIRECT)

	t.Run("api needs the secret and reports a version", func(t *testing.T) {
		if _, code := e.req("GET", "/version", nil, false); code != 401 {
			t.Fatalf("/version without the secret answered %d, want 401", code)
		}
		b, code := e.req("GET", "/version", nil, true)
		var v struct {
			Version string `json:"version"`
		}
		if code != 200 || json.Unmarshal(b, &v) != nil || v.Version == "" {
			t.Fatalf("/version: %d %s", code, b)
		}
	})

	t.Run("groups NetMerge steers are there and can be pointed at a line", func(t *testing.T) {
		p := e.proxies()
		for _, n := range []string{"LINK1", "LINK2", "AUTO", "CALL", "FAST", "STABLE"} {
			if _, ok := p[n]; !ok {
				t.Fatalf("group or line %s missing from /proxies", n)
			}
		}
		for i := 1; i <= dispatchSlots; i++ {
			n := fmt.Sprintf("SLOT%d", i)
			if g, ok := p[n]; !ok || g.Now == "" {
				t.Fatalf("%s missing or has no current line", n)
			}
		}
		for i := 1; i <= dispatchSlots; i++ {
			if _, code := e.req("PUT", fmt.Sprintf("/proxies/SLOT%d", i), map[string]string{"name": "LINK2"}, true); code != 204 && code != 200 {
				t.Fatalf("pointing SLOT%d at LINK2 answered %d", i, code)
			}
		}
		if now := e.proxies()["SLOT1"].Now; now != "LINK2" {
			t.Fatalf("SLOT1 points at %q after setting LINK2", now)
		}
		if _, code := e.req("PUT", "/proxies/CALL", map[string]string{"name": "LINK1"}, true); code != 204 && code != 200 {
			t.Fatalf("pointing CALL answered %d", code)
		}
		if _, code := e.req("PUT", "/proxies/SLOT1", map[string]string{"name": "NO-SUCH-LINE"}, true); code >= 200 && code < 300 {
			t.Fatalf("an unknown line was accepted (%d)", code)
		}
		// every slot on the loopback line, so traffic below can reach the test server
		for i := 1; i <= dispatchSlots; i++ {
			e.req("PUT", fmt.Sprintf("/proxies/SLOT%d", i), map[string]string{"name": "LINK1"}, true)
		}
	})

	t.Run("sharing proxy asks for the password", func(t *testing.T) {
		resp, err := e.httpVia("http://"+local+"/hello", false)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 407 {
			t.Fatalf("without a password the proxy answered %d, want 407", resp.StatusCode)
		}
	})

	t.Run("traffic goes out on the chosen line and shows in the connection list", func(t *testing.T) {
		resp, err := e.httpVia("http://"+local+"/slow", true)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("through the proxy: %d", resp.StatusCode)
		}
		io.CopyN(io.Discard, resp.Body, 256<<10)
		var seen *mConn
		for i := 0; i < 20 && seen == nil; i++ {
			for _, c := range e.connections() {
				if c.Metadata.Host == "localhost" && c.Metadata.DestPort == port {
					c := c
					seen = &c
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		if seen == nil {
			t.Fatal("the open connection is not in /connections")
		}
		if seen.ID == "" || seen.Download <= 0 || seen.Metadata.SrcIP == "" || seen.Metadata.Type == "" {
			t.Fatalf("connection lacks id, download count, source or type: %+v", *seen)
		}
		if !strings.Contains(strings.Join(seen.Chains, ","), "LINK1") || !strings.Contains(strings.Join(seen.Chains, ","), "AUTO") {
			t.Fatalf("chains %v should name the line (LINK1) and the group (AUTO)", seen.Chains)
		}
		if n, _ := io.Copy(io.Discard, resp.Body); n == 0 {
			t.Fatal("the download did not finish")
		}
	})

	t.Run("blocked websites are refused and counted; allowed ones are not", func(t *testing.T) {
		before, _, _, _ := bstats.snapshot()
		if e.connectVia("ads.blocked.test:443") {
			t.Fatal("a blocked website was let through")
		}
		e.connectVia("www.allowed.test:443")
		var after int
		for i := 0; i < 40; i++ {
			after, _, _, _ = bstats.snapshot()
			if after > before {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if after != before+1 {
			t.Fatalf("blocked today went from %d to %d, want exactly one more", before, after)
		}
		log := e.log()
		if !regexp.MustCompile(`ads\.blocked\.test:443 match .* using REJECT`).MatchString(log) {
			t.Fatal("no REJECT line for the blocked website")
		}
		if regexp.MustCompile(`allowed\.test:443 match .* using REJECT`).MatchString(log) {
			t.Fatal("the allowed website was refused")
		}
	})

	t.Run("shadowsocks sharing carries traffic", func(t *testing.T) {
		got, err := ssGet(e.ss, e.pass, "localhost", port, "/hello")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, "hello from the contract server") {
			t.Fatalf("shadowsocks answer: %.200q", got)
		}
	})

	t.Run("settings can be reloaded while running", func(t *testing.T) {
		if _, code := e.req("PUT", "/configs?force=true", map[string]string{"path": filepath.Join(e.dir, "config.yaml")}, true); code != 204 && code != 200 {
			t.Fatalf("reload answered %d", code)
		}
		if _, ok := e.proxies()["AUTO"]; !ok {
			t.Fatal("groups gone after reload")
		}
		resp, err := e.httpVia("http://"+local+"/hello", true)
		if err != nil || resp.StatusCode != 200 {
			t.Fatalf("traffic after reload: %v %v", err, resp)
		}
		resp.Body.Close()
	})
}

// ---- a minimal Shadowsocks (AEAD, aes-128-gcm) client: what a phone sends to the sharing port ----

func ssKey(password string, size int) []byte {
	var key, prev []byte
	for len(key) < size {
		h := md5.New()
		h.Write(prev)
		h.Write([]byte(password))
		prev = h.Sum(nil)
		key = append(key, prev...)
	}
	return key[:size]
}

func ssAEAD(key, salt []byte) cipher.AEAD {
	sub, _ := hkdf.Key(sha1.New, key, salt, "ss-subkey", len(key))
	b, _ := aes.NewCipher(sub)
	a, _ := cipher.NewGCM(b)
	return a
}

func ssGet(server, password, host, port, path string) (string, error) {
	if shareCipher != "aes-128-gcm" {
		return "", fmt.Errorf("test client speaks aes-128-gcm, NetMerge uses %s", shareCipher)
	}
	c, err := net.DialTimeout("tcp", server, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	key := ssKey(password, 16)
	salt := make([]byte, 16)
	rand.Read(salt)
	enc, nonce := ssAEAD(key, salt), make([]byte, 12)
	seal := func(p []byte) []byte {
		out := enc.Seal(nil, nonce, p, nil)
		incNonce(nonce)
		return out
	}
	var addr []byte
	p, _ := net.LookupPort("tcp", port)
	addr = append(addr, 3, byte(len(host)))
	addr = append(addr, host...)
	addr = binary.BigEndian.AppendUint16(addr, uint16(p))
	payload := append(addr, []byte("GET "+path+" HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n\r\n")...)
	var frame []byte
	frame = append(frame, salt...)
	lenb := binary.BigEndian.AppendUint16(nil, uint16(len(payload)))
	frame = append(frame, seal(lenb)...)
	frame = append(frame, seal(payload)...)
	if _, err := c.Write(frame); err != nil {
		return "", err
	}
	rsalt := make([]byte, 16)
	if _, err := io.ReadFull(c, rsalt); err != nil {
		return "", fmt.Errorf("no answer from the shadowsocks port: %w", err)
	}
	dec, rnonce := ssAEAD(key, rsalt), make([]byte, 12)
	var got bytes.Buffer
	for {
		hdr := make([]byte, 2+16)
		if _, err := io.ReadFull(c, hdr); err != nil {
			break
		}
		lb, err := dec.Open(nil, rnonce, hdr, nil)
		if err != nil {
			return got.String(), err
		}
		incNonce(rnonce)
		body := make([]byte, int(binary.BigEndian.Uint16(lb))+16)
		if _, err := io.ReadFull(c, body); err != nil {
			break
		}
		pl, err := dec.Open(nil, rnonce, body, nil)
		if err != nil {
			return got.String(), err
		}
		incNonce(rnonce)
		got.Write(pl)
	}
	return got.String(), nil
}

func incNonce(n []byte) {
	for i := range n {
		n[i]++
		if n[i] != 0 {
			return
		}
	}
}
