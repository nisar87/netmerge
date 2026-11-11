package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestRuleParsingAndDescriptions(t *testing.T) {
	sets := map[string]*DomainSet{"nm_block": NewDomainSet(), "nm_allow": NewDomainSet()}
	sets["nm_block"].Add("+.ads.example")
	sets["nm_block"].Add("+.ok.example")
	sets["nm_allow"].Add("+.ok.example")
	rules, err := ParseRules([]string{
		"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
		`PROCESS-NAME-REGEX,(?i)^swgvc\.exe$,LINK1`,
		"DST-PORT,500,LINK1",
		"AND,((RULE-SET,nm_block),(NOT,((RULE-SET,nm_allow)))),REJECT",
		"DOMAIN-SUFFIX,bank.example,STABLE",
		"DOMAIN-KEYWORD,bank,STABLE",
		"MATCH,AUTO",
	}, sets)
	if err != nil {
		t.Fatal(err)
	}
	// the exact text NetMerge's block counter looks for (blocking.go)
	if d := rules[3].Describe(); d != "AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow)))))" {
		t.Fatalf("AND rule is written as %q", d)
	}
	if d := rules[0].Describe(); d != "IPCIDR(192.168.0.0/16)" {
		t.Fatalf("IP rule is written as %q", d)
	}
	if rules[6].Describe() != "Match" {
		t.Fatal("MATCH")
	}
	env := &ruleEnv{process: func(m *Metadata) { m.processDone = true; m.Process = "SWGVC.EXE" }}
	cases := []struct {
		m    Metadata
		want int
	}{
		{Metadata{DstIP: netip.MustParseAddr("192.168.1.5"), DstPort: 80}, 0},
		{Metadata{Host: "x.example", DstPort: 443, SrcIP: netip.MustParseAddr("198.18.0.1")}, 1},
		{Metadata{SniffHost: "img.ads.example", DstPort: 500}, 2},
	}
	for i, c := range cases {
		m := c.m
		e := env
		if i != 1 {
			e = &ruleEnv{}
		}
		got := -1
		for j, r := range rules {
			if r.cond.match(&m, e) {
				got = j
				break
			}
		}
		if got != c.want {
			t.Fatalf("case %d matched rule %d, want %d", i, got, c.want)
		}
	}
	match := func(host string) int {
		m := Metadata{SniffHost: host, DstPort: 443}
		for j, r := range rules {
			if r.cond.match(&m, &ruleEnv{}) {
				return j
			}
		}
		return -1
	}
	if match("img.ads.example") != 3 || match("www.ok.example") != 6 || match("mybank.com") != 5 || match("www.bank.example") != 4 {
		t.Fatalf("block/allow/bank rules: %d %d %d %d", match("img.ads.example"), match("www.ok.example"), match("mybank.com"), match("www.bank.example"))
	}
	for _, bad := range []string{"AND,((RULE-SET,nm_block),REJECT", "RULE-SET,missing,REJECT", "FOO,bar,DIRECT", "DST-PORT,x,DIRECT"} {
		if _, err := ParseRules([]string{bad}, sets); err == nil {
			t.Fatalf("%q should be refused", bad)
		}
	}
}

func TestDomainSet(t *testing.T) {
	d := NewDomainSet()
	for _, l := range []string{"+.tree.test", ".sub.test", "*.wild.test", "exact.test", "# comment", ""} {
		d.Add(l)
	}
	yes := []string{"tree.test", "a.b.tree.test", "x.sub.test", "y.x.sub.test", "one.wild.test", "exact.test", "EXACT.test."}
	no := []string{"sub.test", "two.one.wild.test", "wild.test", "a.exact.test", "notree.test", ""}
	for _, h := range yes {
		if !d.Has(h) {
			t.Fatalf("%q should match", h)
		}
	}
	for _, h := range no {
		if d.Has(h) {
			t.Fatalf("%q should not match", h)
		}
	}
}

func TestSniffHTTPAndTLS(t *testing.T) {
	if h := sniffHTTP([]byte("GET /x HTTP/1.1\r\nHost: Example.COM:8080\r\nAccept: */*\r\n\r\n")); h != "example.com" {
		t.Fatalf("HTTP host %q", h)
	}
	if h := sniffHTTP([]byte("\x16\x03\x01junk")); h != "" {
		t.Fatal("TLS bytes read as HTTP")
	}
	// a real ClientHello, written by Go's TLS client
	c1, c2 := net.Pipe()
	go func() {
		tls.Client(c1, &tls.Config{ServerName: "www.netmerge.example", InsecureSkipVerify: true}).Handshake()
	}()
	buf := make([]byte, 4096)
	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := c2.Read(buf)
	c2.Close()
	c1.Close()
	if h := sniffTLS(buf[:n]); h != "www.netmerge.example" {
		t.Fatalf("TLS server name %q", h)
	}
}

// RFC 9001, Appendix A.1: keys for the client's first packet, from connection ID 0x8394c8f03e515708.
func TestQUICInitialKeys(t *testing.T) {
	dcid, _ := hex.DecodeString("8394c8f03e515708")
	initial, _ := hkdf.Extract(sha256.New, dcid, quicV1Salt)
	client := hkdfExpandLabel(initial, "client in", 32)
	want := map[string]string{
		"secret": "c00cf151ca5be075ed0ebfb5c80323c42d6b7db67881289af4008f1f6c357aea",
		"key":    "1f369613dd76d5467730efcbe3b1a22d",
		"iv":     "fa044b2f42a3fd3b46fb255c",
		"hp":     "9f50449e04a0e810283a1e9933adedd2",
	}
	got := map[string]string{
		"secret": hex.EncodeToString(client),
		"key":    hex.EncodeToString(hkdfExpandLabel(client, "quic key", 16)),
		"iv":     hex.EncodeToString(hkdfExpandLabel(client, "quic iv", 12)),
		"hp":     hex.EncodeToString(hkdfExpandLabel(client, "quic hp", 16)),
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %s, RFC 9001 says %s", k, got[k], v)
		}
	}
}

// A QUIC Initial built the way a browser builds it must give back its server name.
func TestSniffQUIC(t *testing.T) {
	ch := tlsClientHelloRecord(t, "video.netmerge.example")[5:] // handshake message without the record header
	var frames []byte
	frames = append(frames, 0x06, 0x00)                           // CRYPTO, offset 0
	frames = append(frames, byte(0x40|len(ch)>>8), byte(len(ch))) // 2-byte length
	frames = append(frames, ch...)
	for len(frames) < 1162 {
		frames = append(frames, 0) // PADDING
	}
	dcid := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	pn := []byte{0, 0, 0, 2}
	hdr := []byte{0xc3, 0, 0, 0, 1, byte(len(dcid))}
	hdr = append(hdr, dcid...)
	hdr = append(hdr, 0, 0) // no source id, no token
	length := len(pn) + len(frames) + 16
	hdr = append(hdr, byte(0x40|length>>8), byte(length))
	pnOff := len(hdr)
	hdr = append(hdr, pn...)
	initial, _ := hkdf.Extract(sha256.New, dcid, quicV1Salt)
	client := hkdfExpandLabel(initial, "client in", 32)
	key, iv, hp := hkdfExpandLabel(client, "quic key", 16), hkdfExpandLabel(client, "quic iv", 12), hkdfExpandLabel(client, "quic hp", 16)
	kb, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(kb)
	nonce := append([]byte(nil), iv...)
	nonce[11] ^= 2
	pkt := append(append([]byte(nil), hdr...), aead.Seal(nil, nonce, frames, hdr)...)
	hb, _ := aes.NewCipher(hp)
	mask := make([]byte, 16)
	hb.Encrypt(mask, pkt[pnOff+4:pnOff+20])
	pkt[0] ^= mask[0] & 0x0f
	for i := 0; i < 4; i++ {
		pkt[pnOff+i] ^= mask[1+i]
	}
	if h := sniffQUIC(pkt); h != "video.netmerge.example" {
		t.Fatalf("QUIC server name %q", h)
	}
	if sniffQUIC(pkt[:100]) != "" {
		t.Fatal("a cut packet gave a name")
	}
}

func tlsClientHelloRecord(t *testing.T, name string) []byte {
	c1, c2 := net.Pipe()
	go func() { tls.Client(c1, &tls.Config{ServerName: name, InsecureSkipVerify: true}).Handshake() }()
	buf := make([]byte, 8192)
	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c2.Read(buf)
	c2.Close()
	c1.Close()
	if err != nil {
		t.Fatal(err)
	}
	return buf[:n]
}

func TestDNSMessages(t *testing.T) {
	q := buildQuery("www.example.com", 1)
	name, qt, qend, ok := parseQuestion(q)
	if !ok || name != "www.example.com" || qt != 1 || qend != len(q) {
		t.Fatalf("question: %q %d %d %v", name, qt, qend, ok)
	}
	a := netip.MustParseAddr("198.18.0.9")
	r := buildReply(q, []netip.Addr{a}, 1)
	got, ttl := parseAnswers(r, 1)
	if len(got) != 1 || got[0] != a || ttl != 1 {
		t.Fatalf("answers %v ttl %d", got, ttl)
	}
	if !bytes.Equal(r[:2], q[:2]) || r[2]&0x80 == 0 {
		t.Fatal("reply must keep the id and be marked as a response")
	}
	if as, _ := parseAnswers(buildServFail(q), 1); len(as) != 0 {
		t.Fatal("SERVFAIL has no answers")
	}
}

func TestFakePool(t *testing.T) {
	p := newFakePool(netip.MustParsePrefix("198.18.0.1/16"))
	a := p.get("a.example")
	if a.String() != "198.18.0.3" || p.get("a.example") != a {
		t.Fatalf("first address %s (198.18.0.1 and .2 belong to the adapter)", a)
	}
	b := p.get("b.example")
	if h, ok := p.host(b); !ok || h != "b.example" || a == b {
		t.Fatal("reverse lookup")
	}
}

func TestPacketRewriteKeepsChecksumsRight(t *testing.T) {
	src := netip.MustParseAddrPort("198.18.0.1:40000")
	dst := netip.MustParseAddrPort("203.0.113.10:5353")
	pkt := buildUDP(src, dst, []byte("hello"))
	p, ok := parseIP(pkt)
	if !ok || p.proto != protoUDP || p.src != src.Addr() {
		t.Fatal("parse")
	}
	setAddrs(&p, netip.MustParseAddrPort("198.18.0.2:10001"), netip.MustParseAddrPort("198.18.0.1:5555"))
	// a correct IPv4 header and UDP checksum sum to 0xffff
	if csumFold(csumAdd(0, p.b[:20])) != 0 {
		t.Fatal("IPv4 header checksum")
	}
	sum := pseudoSum(p.src, p.dst, protoUDP, len(p.payload))
	if csumFold(csumAdd(sum, p.payload)) != 0 {
		t.Fatal("UDP checksum")
	}
	if binary.BigEndian.Uint16(p.payload[2:4]) != 5555 {
		t.Fatal("port")
	}
}

func TestNATTable(t *testing.T) {
	n := newNATTable()
	app, dst := netip.MustParseAddrPort("198.18.0.1:5000"), netip.MustParseAddrPort("1.2.3.4:443")
	if n.lookup(app, dst, false) != nil {
		t.Fatal("a non-SYN packet must not open an entry")
	}
	en := n.lookup(app, dst, true)
	if en == nil || n.lookup(app, dst, false) != en || n.byPort(en.port) != en {
		t.Fatal("entry")
	}
	other := n.lookup(netip.MustParseAddrPort("198.18.0.1:5001"), dst, true)
	if other.port == en.port {
		t.Fatal("two connections share a port")
	}
}

func TestRouterPlanAndFallbackOrder(t *testing.T) {
	c := &Config{
		Proxies: []ProxyConfig{{Name: "LINK1", Type: "direct", InterfaceName: "a"}, {Name: "LINK1#2", Type: "direct", InterfaceName: "a"}, {Name: "LINK2", Type: "direct", InterfaceName: "b"}},
		Groups: []GroupConfig{
			{Name: "SLOT1", Type: "select", Proxies: []string{"LINK1", "LINK2"}},
			{Name: "SLOT2", Type: "select", Proxies: []string{"LINK1", "LINK2"}},
			{Name: "AUTO", Type: "load-balance", Strategy: "round-robin", Proxies: []string{"SLOT1", "SLOT2"}},
			{Name: "STABLE", Type: "load-balance", Strategy: "consistent-hashing", Proxies: []string{"LINK1", "LINK1#2", "LINK2"}},
		},
	}
	r, err := newRouter(c)
	if err != nil {
		t.Fatal(err)
	}
	r.outs["SLOT2"].(*Selector).Set("LINK2")
	m := &Metadata{SniffHost: "www.site.example", DstPort: 443}
	p1, _ := r.resolvePlan("AUTO", m)
	p2, _ := r.resolvePlan("AUTO", m)
	if p1.lines[0].name != "LINK1" || p2.lines[0].name != "LINK2" || strings.Join(p1.chain, ",") != "SLOT1,AUTO" {
		t.Fatalf("round-robin over slots: %s %s %v", p1.lines[0].name, p2.lines[0].name, p1.chain)
	}
	if len(p1.lines) != 2 {
		t.Fatalf("fallbacks: %d lines", len(p1.lines))
	}
	// the same website always lands on the same line
	s1, _ := r.resolvePlan("STABLE", m)
	s2, _ := r.resolvePlan("STABLE", &Metadata{SniffHost: "img.site.example", DstPort: 443})
	if s1.lines[0].Base() != s2.lines[0].Base() {
		t.Fatal("consistent hashing should keep one site on one line")
	}
	// a line that just failed goes to the back
	p1.lines[0].markFailed()
	p3, _ := r.resolvePlan("SLOT1", m)
	if p3.lines[0].name != "LINK2" {
		t.Fatalf("failed line first: %s", p3.lines[0].name)
	}
	// choices survive a reload
	r2, _ := newRouter(c)
	r2.keepChoices(r)
	if r2.outs["SLOT2"].(*Selector).Now() != "LINK2" {
		t.Fatal("slot choice lost on reload")
	}
	if !r2.outs["SLOT1"].(*Selector).Set("LINK2") || r2.outs["SLOT1"].(*Selector).Set("NOPE") {
		t.Fatal("selector accepts only its members")
	}
}
