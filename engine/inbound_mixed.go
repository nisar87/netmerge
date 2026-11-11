package engine

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// The sharing port: HTTP proxy and SOCKS5 on one port (what phones are told to use), with a password.

type mixedListener struct {
	e  *Engine
	l  net.Listener
	ul *net.UDPConn // SOCKS5 UDP relay
}

func (e *Engine) startMixed(addr string) (io.Closer, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	ml := &mixedListener{e: e, l: l}
	go ml.serve()
	return ml, nil
}

func (ml *mixedListener) Close() error {
	if ml.ul != nil {
		ml.ul.Close()
	}
	return ml.l.Close()
}

func (ml *mixedListener) serve() {
	for {
		c, err := ml.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go ml.handle(c)
	}
}

// allowedSource: with "allow-lan" off only this PC may connect; with it on, the addresses in lan-allowed-ips.
func (e *Engine) allowedSource(a net.Addr) bool {
	ap, err := netip.ParseAddrPort(a.String())
	if err != nil {
		return false
	}
	ip := ap.Addr().Unmap()
	if ip.IsLoopback() {
		return true
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !e.cfg.AllowLAN {
		return false
	}
	if len(e.lanNets) == 0 {
		return ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	for _, p := range e.lanNets {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func (e *Engine) needsAuth() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.authOK) > 0
}

func (e *Engine) checkAuth(user, pass string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.authOK) == 0 || e.authOK[user+":"+pass]
}

func srcMeta(c net.Conn, typ string) *Metadata {
	m := &Metadata{Type: typ}
	if ap, err := netip.ParseAddrPort(c.RemoteAddr().String()); err == nil {
		m.SrcIP, m.SrcPort = ap.Addr().Unmap(), ap.Port()
	}
	return m
}

func (ml *mixedListener) handle(c net.Conn) {
	if !ml.e.allowedSource(c.RemoteAddr()) {
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(c)
	first, err := br.Peek(1)
	if err != nil {
		c.Close()
		return
	}
	if first[0] == 5 {
		ml.socks5(&peekedConn{Conn: c, r: br}, br)
		return
	}
	ml.httpProxy(c, br)
}

// setDest fills the destination from "host:port".
func setDest(m *Metadata, hostport string) error {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(p)
	if err != nil || port <= 0 || port > 65535 {
		return fmt.Errorf("bad port %q", p)
	}
	m.DstPort = uint16(port)
	if a, err := netip.ParseAddr(h); err == nil {
		m.DstIP = a.Unmap()
		m.Host = ""
	} else {
		m.Host = strings.ToLower(strings.TrimSuffix(h, "."))
	}
	return nil
}

// ---- HTTP proxy ----

func proxyAuth(r *http.Request) (string, string) {
	v := r.Header.Get("Proxy-Authorization")
	if !strings.HasPrefix(v, "Basic ") {
		return "", ""
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, "Basic "))
	if err != nil {
		return "", ""
	}
	u, p, _ := strings.Cut(string(b), ":")
	return u, p
}

func (ml *mixedListener) httpProxy(c net.Conn, br *bufio.Reader) {
	e := ml.e
	defer c.Close()
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if e.needsAuth() {
		if u, p := proxyAuth(req); !e.checkAuth(u, p) {
			io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nProxy-Authenticate: Basic realm=\"NetMerge\"\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			return
		}
	}
	_ = c.SetReadDeadline(time.Time{})
	if req.Method == http.MethodConnect {
		m := srcMeta(c, "HTTPS")
		if err := setDest(m, req.Host); err != nil {
			io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
			return
		}
		// the tunnel is confirmed only once the website answered (or refused with an honest error)
		e.handleTCPReady(&peekedConn{Conn: c, r: br}, m, func(err error, rejected bool) bool {
			switch {
			case rejected:
				io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return false
			case err != nil:
				io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return false
			}
			_, werr := io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
			return werr == nil
		})
		return
	}
	ml.plainHTTP(c, br, req)
}

// plainHTTP forwards ordinary http:// requests. One connection from the phone may ask several websites in turn;
// each request goes to its own website (and line), and the connection stays open between requests.
func (ml *mixedListener) plainHTTP(c net.Conn, br *bufio.Reader, req *http.Request) {
	e := ml.e
	var out net.Conn
	var outKey string
	var outBR *bufio.Reader
	var tr *tracked
	defer func() {
		if out != nil {
			out.Close()
		}
		if tr != nil {
			e.tracker.remove(tr)
		}
	}()
	for {
		if req.URL.Host == "" {
			req.URL.Host = req.Host
		}
		hostport := req.URL.Host
		if !strings.Contains(hostport, ":") || strings.HasSuffix(hostport, "]") {
			hostport = net.JoinHostPort(strings.Trim(hostport, "[]"), "80")
		}
		if out == nil || outKey != hostport {
			if out != nil {
				out.Close()
				e.tracker.remove(tr)
				out, tr = nil, nil
			}
			m := srcMeta(c, "HTTP")
			m.Network = "tcp"
			if err := setDest(m, hostport); err != nil {
				io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return
			}
			rule, p, err := e.decide(m)
			if err != nil || p.reject {
				if p != nil && p.reject {
					e.log.Infof("[TCP] %s --> %s match %s using REJECT", srcText(m), m.RemoteAddress(), rule.Describe())
				}
				io.WriteString(c, "HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return
			}
			conn, used, err := e.dial.dialTCP(req.Context(), p, m)
			if err != nil {
				e.log.Warnf("[TCP] dial %s (match %s/%s) %s --> %s error: %v", usingText(p, nil), rule.Name(), rule.Payload(), srcText(m), m.RemoteAddress(), err)
				io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
				return
			}
			e.log.Infof("[TCP] %s --> %s match %s using %s", srcText(m), m.RemoteAddress(), rule.Describe(), usingText(p, used))
			out, outKey, outBR = conn, hostport, bufio.NewReader(conn)
			tr = e.tracker.add(m, append([]string{used.Base()}, p.chain...), rule, multiCloser{c, conn})
		}
		// origin form, without the proxy's own headers
		req.RequestURI = ""
		req.Header.Del("Proxy-Authorization")
		req.Header.Del("Proxy-Connection")
		upgrade := strings.EqualFold(req.Header.Get("Connection"), "upgrade")
		cw := &countWriter{w: out, n: func(n int64) { e.tracker.addUp(tr, n) }}
		if err := req.Write(cw); err != nil {
			return
		}
		resp, err := http.ReadResponse(outBR, req)
		if err != nil {
			return
		}
		rw := &countWriter{w: c, n: func(n int64) { e.tracker.addDown(tr, n) }}
		if resp.StatusCode == http.StatusSwitchingProtocols && upgrade {
			_ = resp.Write(rw)
			// websocket or similar over plain HTTP: from here on both sides talk freely
			e.relay(&peekedConn{Conn: c, r: br}, &peekedConn{Conn: out, r: outBR}, tr)
			out = nil
			return
		}
		if err := resp.Write(rw); err != nil {
			resp.Body.Close()
			return
		}
		resp.Body.Close()
		if resp.Close || req.Close {
			return
		}
		next, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		req = next
	}
}

type countWriter struct {
	w io.Writer
	n func(int64)
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	if n > 0 {
		c.n(int64(n))
	}
	return n, err
}

// ---- SOCKS5 ----

func socksReadAddr(r io.Reader) (string, error) {
	var t [1]byte
	if _, err := io.ReadFull(r, t[:]); err != nil {
		return "", err
	}
	var host string
	switch t[0] {
	case 1:
		var b [4]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = netip.AddrFrom4(b).String()
	case 4:
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return "", err
		}
		host = netip.AddrFrom16(b).String()
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(r, l[:]); err != nil {
			return "", err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(r, b); err != nil {
			return "", err
		}
		host = string(b)
	default:
		return "", fmt.Errorf("address type %d", t[0])
	}
	var p [2]byte
	if _, err := io.ReadFull(r, p[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(p[:])))), nil
}

// socksAddr encodes host:port in the SOCKS form (also used by Shadowsocks).
func socksAddr(ap netip.AddrPort, host string) []byte {
	var b []byte
	switch {
	case host != "":
		b = append(b, 3, byte(len(host)))
		b = append(b, host...)
	case ap.Addr().Is4() || ap.Addr().Is4In6():
		a := ap.Addr().Unmap().As4()
		b = append(b, 1)
		b = append(b, a[:]...)
	default:
		a := ap.Addr().As16()
		b = append(b, 4)
		b = append(b, a[:]...)
	}
	return binary.BigEndian.AppendUint16(b, ap.Port())
}

func (ml *mixedListener) socks5(c net.Conn, br *bufio.Reader) {
	e := ml.e
	fail := func(code byte) {
		c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
		c.Close()
	}
	var hdr [2]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil {
		c.Close()
		return
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(br, methods); err != nil {
		c.Close()
		return
	}
	want := byte(0)
	if e.needsAuth() {
		want = 2
	}
	ok := false
	for _, m := range methods {
		if m == want {
			ok = true
		}
	}
	if !ok {
		c.Write([]byte{5, 0xff})
		c.Close()
		return
	}
	c.Write([]byte{5, want})
	if want == 2 {
		var v [2]byte
		if _, err := io.ReadFull(br, v[:]); err != nil {
			c.Close()
			return
		}
		u := make([]byte, v[1])
		io.ReadFull(br, u)
		var pl [1]byte
		io.ReadFull(br, pl[:])
		p := make([]byte, pl[0])
		io.ReadFull(br, p)
		if !e.checkAuth(string(u), string(p)) {
			c.Write([]byte{1, 1})
			c.Close()
			return
		}
		c.Write([]byte{1, 0})
	}
	var req [3]byte
	if _, err := io.ReadFull(br, req[:]); err != nil {
		c.Close()
		return
	}
	dst, err := socksReadAddr(br)
	if err != nil {
		fail(8)
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	switch req[1] {
	case 1: // CONNECT
		m := srcMeta(c, "Socks5")
		if err := setDest(m, dst); err != nil {
			fail(8)
			return
		}
		e.handleTCPReady(c, m, func(err error, rejected bool) bool {
			if rejected {
				fail(2)
				return false
			}
			if err != nil {
				fail(5)
				return false
			}
			_, werr := c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
			return werr == nil
		})
	case 3: // UDP ASSOCIATE: datagrams go to our UDP port, wrapped in a SOCKS header
		ua, err := ml.udpRelay()
		if err != nil {
			fail(1)
			return
		}
		local := c.LocalAddr().(*net.TCPAddr)
		ap := netip.AddrPortFrom(netip.MustParseAddr(local.IP.String()).Unmap(), uint16(ua.Port))
		reply := append([]byte{5, 0, 0}, socksAddr(ap, "")...)
		c.Write(reply)
		// the association lasts as long as this TCP connection
		io.Copy(io.Discard, c)
		c.Close()
	default:
		fail(7)
	}
}

// udpRelay opens (once) the UDP port for SOCKS5 UDP ASSOCIATE.
func (ml *mixedListener) udpRelay() (*net.UDPAddr, error) {
	if ml.ul != nil {
		return ml.ul.LocalAddr().(*net.UDPAddr), nil
	}
	la := ml.l.Addr().(*net.TCPAddr)
	u, err := net.ListenUDP("udp", &net.UDPAddr{IP: la.IP, Port: 0})
	if err != nil {
		return nil, err
	}
	ml.ul = u
	go func() {
		buf := make([]byte, 65535)
		for {
			n, from, err := u.ReadFromUDPAddrPort(buf)
			if err != nil {
				return
			}
			if !ml.e.allowedSource(net.UDPAddrFromAddrPort(from)) || n < 4 || buf[2] != 0 { // no fragments
				continue
			}
			r := bufio.NewReader(strings.NewReader(string(buf[3:n])))
			dst, err := socksReadAddr(r)
			if err != nil {
				continue
			}
			payload, _ := io.ReadAll(r)
			m := &Metadata{Type: "Socks5", Network: "udp", SrcIP: from.Addr().Unmap(), SrcPort: from.Port()}
			if setDest(m, dst) != nil {
				continue
			}
			client := from
			ml.e.udp.send(m, payload, "socks|"+from.String(), func(p []byte, src netip.AddrPort, host string) error {
				pkt := append([]byte{0, 0, 0}, socksAddr(src, "")...)
				pkt = append(pkt, p...)
				_, err := u.WriteToUDPAddrPort(pkt, client)
				return err
			})
		}
	}()
	return u.LocalAddr().(*net.UDPAddr), nil
}
