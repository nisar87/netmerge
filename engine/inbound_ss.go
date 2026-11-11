package engine

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Shadowsocks (AEAD) sharing port: what the phone app connects to. Ciphers: aes-128-gcm (NetMerge's) and aes-256-gcm.

type ssListener struct {
	e     *Engine
	l     net.Listener
	u     *net.UDPConn
	key   []byte
	salts *saltFilter
}

func ssKeySize(cipherName string) (int, error) {
	switch cipherName {
	case "aes-128-gcm":
		return 16, nil
	case "aes-256-gcm":
		return 32, nil
	}
	return 0, fmt.Errorf("cipher %q is not supported (use aes-128-gcm or aes-256-gcm)", cipherName)
}

// evpBytesToKey turns the password into the master key, as every Shadowsocks client does.
func evpBytesToKey(password string, size int) []byte {
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

func ssAEADFor(key, salt []byte) (cipher.AEAD, error) {
	sub, err := hkdf.Key(sha1.New, key, salt, "ss-subkey", len(key))
	if err != nil {
		return nil, err
	}
	b, err := aes.NewCipher(sub)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func (e *Engine) startSS(lc ListenerConfig) (io.Closer, error) {
	size, err := ssKeySize(lc.Cipher)
	if err != nil {
		return nil, err
	}
	if lc.Password == "" {
		return nil, errors.New("shadowsocks needs a password")
	}
	listen := lc.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}
	addr := net.JoinHostPort(listen, fmt.Sprint(lc.Port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &ssListener{e: e, l: l, key: evpBytesToKey(lc.Password, size), salts: newSaltFilter()}
	if lc.UDP {
		ua, _ := net.ResolveUDPAddr("udp", addr)
		if u, err := net.ListenUDP("udp", ua); err == nil {
			s.u = u
			go s.serveUDP()
		} else {
			e.log.Warnf("shadowsocks UDP on %s: %v", addr, err)
		}
	}
	go s.serve()
	return s, nil
}

func (s *ssListener) Close() error {
	if s.u != nil {
		s.u.Close()
	}
	return s.l.Close()
}

func (s *ssListener) serve() {
	for {
		c, err := s.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		go s.handle(c)
	}
}

func (s *ssListener) handle(c net.Conn) {
	if !s.e.allowedSource(c.RemoteAddr()) {
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	sc, err := newSSConn(c, s.key, s.salts)
	if err != nil {
		// wrong password or a replay: say nothing (like a closed port) after a short random wait
		time.Sleep(time.Duration(200+int(time.Now().UnixNano()%800)) * time.Millisecond)
		c.Close()
		return
	}
	dst, err := socksReadAddr(sc)
	if err != nil {
		c.Close()
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	m := srcMeta(c, "Shadowsocks")
	if err := setDest(m, dst); err != nil {
		c.Close()
		return
	}
	s.e.handleTCP(sc, m)
}

// ssConn is one Shadowsocks TCP stream: reading opens the client's chunks, writing seals ours.
type ssConn struct {
	net.Conn
	key  []byte
	rd   cipher.AEAD
	rn   []byte
	rbuf []byte
	wr   cipher.AEAD
	wn   []byte
	wmu  sync.Mutex
	br   *bufio.Reader
}

func newSSConn(c net.Conn, key []byte, salts *saltFilter) (*ssConn, error) {
	br := bufio.NewReaderSize(c, 32*1024)
	salt := make([]byte, len(key))
	if _, err := io.ReadFull(br, salt); err != nil {
		return nil, err
	}
	if !salts.add(salt) {
		return nil, errors.New("replayed salt")
	}
	rd, err := ssAEADFor(key, salt)
	if err != nil {
		return nil, err
	}
	sc := &ssConn{Conn: c, key: key, rd: rd, rn: make([]byte, rd.NonceSize()), br: br}
	// read the first chunk now: a wrong password fails here
	if err := sc.fill(); err != nil {
		return nil, err
	}
	return sc, nil
}

func ssIncNonce(n []byte) {
	for i := range n {
		n[i]++
		if n[i] != 0 {
			return
		}
	}
}

// fill reads and opens one chunk into rbuf.
func (s *ssConn) fill() error {
	tag := s.rd.Overhead()
	hdr := make([]byte, 2+tag)
	if _, err := io.ReadFull(s.br, hdr); err != nil {
		return err
	}
	lb, err := s.rd.Open(hdr[:0], s.rn, hdr, nil)
	if err != nil {
		return err
	}
	ssIncNonce(s.rn)
	n := int(binary.BigEndian.Uint16(lb)) & 0x3fff
	body := make([]byte, n+tag)
	if _, err := io.ReadFull(s.br, body); err != nil {
		return err
	}
	pl, err := s.rd.Open(body[:0], s.rn, body, nil)
	if err != nil {
		return err
	}
	ssIncNonce(s.rn)
	s.rbuf = pl
	return nil
}

func (s *ssConn) Read(b []byte) (int, error) {
	for len(s.rbuf) == 0 {
		if err := s.fill(); err != nil {
			return 0, err
		}
	}
	n := copy(b, s.rbuf)
	s.rbuf = s.rbuf[n:]
	return n, nil
}

func (s *ssConn) Write(b []byte) (int, error) {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	var out bytes.Buffer
	if s.wr == nil {
		salt := make([]byte, len(s.key))
		if _, err := rand.Read(salt); err != nil {
			return 0, err
		}
		w, err := ssAEADFor(s.key, salt)
		if err != nil {
			return 0, err
		}
		s.wr, s.wn = w, make([]byte, w.NonceSize())
		out.Write(salt)
	}
	written := 0
	for len(b) > 0 {
		n := len(b)
		if n > 0x3fff {
			n = 0x3fff
		}
		lb := binary.BigEndian.AppendUint16(nil, uint16(n))
		out.Write(s.wr.Seal(nil, s.wn, lb, nil))
		ssIncNonce(s.wn)
		out.Write(s.wr.Seal(nil, s.wn, b[:n], nil))
		ssIncNonce(s.wn)
		b = b[n:]
		written += n
	}
	if _, err := s.Conn.Write(out.Bytes()); err != nil {
		return 0, err
	}
	return written, nil
}

func (s *ssConn) CloseWrite() error {
	if cw, ok := s.Conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return nil
}

// ---- UDP: every packet is salt + sealed(address + data) ----

func (s *ssListener) serveUDP() {
	buf := make([]byte, 65535)
	for {
		n, from, err := s.u.ReadFromUDPAddrPort(buf)
		if err != nil {
			return
		}
		if !s.e.allowedSource(net.UDPAddrFromAddrPort(from)) || n < len(s.key)+16 {
			continue
		}
		salt := buf[:len(s.key)]
		a, err := ssAEADFor(s.key, salt)
		if err != nil {
			continue
		}
		plain, err := a.Open(nil, make([]byte, a.NonceSize()), buf[len(s.key):n], nil)
		if err != nil {
			continue
		}
		r := bytes.NewReader(plain)
		dst, err := socksReadAddr(r)
		if err != nil {
			continue
		}
		payload := plain[len(plain)-r.Len():]
		m := &Metadata{Type: "Shadowsocks", Network: "udp", SrcIP: from.Addr().Unmap(), SrcPort: from.Port()}
		if setDest(m, dst) != nil {
			continue
		}
		client := from
		s.e.udp.send(m, append([]byte(nil), payload...), "ss|"+from.String(), func(p []byte, src netip.AddrPort, host string) error {
			salt := make([]byte, len(s.key))
			rand.Read(salt)
			w, err := ssAEADFor(s.key, salt)
			if err != nil {
				return err
			}
			body := append(socksAddr(src, ""), p...)
			pkt := append(salt, w.Seal(nil, make([]byte, w.NonceSize()), body, nil)...)
			_, err = s.u.WriteToUDPAddrPort(pkt, client)
			return err
		})
	}
}

// saltFilter remembers salts seen in the last hour, so recorded traffic cannot be played back.
type saltFilter struct {
	mu   sync.Mutex
	cur  map[string]struct{}
	prev map[string]struct{}
	at   time.Time
}

func newSaltFilter() *saltFilter {
	return &saltFilter{cur: map[string]struct{}{}, prev: map[string]struct{}{}, at: time.Now()}
}

func (f *saltFilter) add(salt []byte) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if time.Since(f.at) > 30*time.Minute || len(f.cur) > 200000 {
		f.prev, f.cur, f.at = f.cur, map[string]struct{}{}, time.Now()
	}
	k := string(salt)
	if _, ok := f.cur[k]; ok {
		return false
	}
	if _, ok := f.prev[k]; ok {
		return false
	}
	f.cur[k] = struct{}{}
	return true
}
