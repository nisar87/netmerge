package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
)

// Finding the website name inside the first bytes of a connection, so rules for names work for connections that
// were made to an address (the PC's own traffic, phones that looked the name up themselves).

// sniffHTTP returns the Host header of a plain HTTP request.
func sniffHTTP(b []byte) string {
	i := bytes.IndexByte(b, ' ')
	if i < 3 || i > 10 {
		return ""
	}
	switch string(b[:i]) {
	case "GET", "POST", "HEAD", "PUT", "DELETE", "OPTIONS", "PATCH", "CONNECT", "TRACE":
	default:
		return ""
	}
	for _, line := range bytes.Split(b, []byte("\r\n"))[1:] {
		if len(line) == 0 {
			break
		}
		k, v, ok := bytes.Cut(line, []byte(":"))
		if ok && strings.EqualFold(string(bytes.TrimSpace(k)), "host") {
			h := strings.TrimSpace(string(v))
			if j := strings.LastIndexByte(h, ':'); j > 0 && !strings.Contains(h[j:], "]") {
				h = h[:j]
			}
			return cleanHost(h)
		}
	}
	return ""
}

// sniffTLS returns the server name (SNI) from a TLS ClientHello.
func sniffTLS(b []byte) string {
	if len(b) < 5 || b[0] != 0x16 || b[1] != 3 {
		return ""
	}
	n := int(binary.BigEndian.Uint16(b[3:5]))
	b = b[5:]
	if len(b) > n {
		b = b[:n]
	}
	return clientHelloSNI(b)
}

// clientHelloSNI reads the handshake message (type 1) and its server_name extension.
func clientHelloSNI(b []byte) string {
	if len(b) < 4 || b[0] != 1 {
		return ""
	}
	b = b[4:]
	if len(b) < 34 {
		return ""
	}
	b = b[34:] // version + random
	skip := func(lenBytes int) bool {
		if len(b) < lenBytes {
			return false
		}
		n := 0
		for i := 0; i < lenBytes; i++ {
			n = n<<8 | int(b[i])
		}
		if len(b) < lenBytes+n {
			return false
		}
		b = b[lenBytes+n:]
		return true
	}
	if !skip(1) || !skip(2) || !skip(1) { // session id, cipher suites, compression
		return ""
	}
	if len(b) < 2 {
		return ""
	}
	ext := b[2:]
	if el := int(binary.BigEndian.Uint16(b)); el < len(ext) {
		ext = ext[:el]
	}
	for len(ext) >= 4 {
		typ, l := binary.BigEndian.Uint16(ext), int(binary.BigEndian.Uint16(ext[2:]))
		if len(ext) < 4+l {
			return ""
		}
		data := ext[4 : 4+l]
		ext = ext[4+l:]
		if typ != 0 || len(data) < 5 {
			continue
		}
		data = data[2:] // list length
		for len(data) >= 3 {
			nt, nl := data[0], int(binary.BigEndian.Uint16(data[1:]))
			if len(data) < 3+nl {
				return ""
			}
			if nt == 0 {
				return cleanHost(string(data[3 : 3+nl]))
			}
			data = data[3+nl:]
		}
	}
	return ""
}

func cleanHost(h string) string {
	h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
	if h == "" || len(h) > 253 {
		return ""
	}
	for _, c := range h {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_') {
			return ""
		}
	}
	return h
}

// ---- QUIC (version 1): the server name is in the ClientHello inside the first, lightly protected packet ----

var quicV1Salt = []byte{0x38, 0x76, 0x2c, 0xf7, 0xf5, 0x59, 0x34, 0xb3, 0x4d, 0x17, 0x9a, 0xe6, 0xa4, 0xc8, 0x0c, 0xad, 0xcc, 0xbb, 0x7f, 0x0a}

// sniffQUIC returns the server name from a QUIC Initial packet, or "".
func sniffQUIC(pkt []byte) string {
	h, err := quicInitialCrypto(pkt)
	if err != nil || len(h) == 0 {
		return ""
	}
	return clientHelloSNI(h)
}

func quicVarint(b []byte) (uint64, int) {
	if len(b) == 0 {
		return 0, 0
	}
	l := 1 << (b[0] >> 6)
	if len(b) < l {
		return 0, 0
	}
	v := uint64(b[0] & 0x3f)
	for i := 1; i < l; i++ {
		v = v<<8 | uint64(b[i])
	}
	return v, l
}

func hkdfExpandLabel(secret []byte, label string, length int) []byte {
	full := "tls13 " + label
	info := make([]byte, 0, 4+len(full))
	info = binary.BigEndian.AppendUint16(info, uint16(length))
	info = append(info, byte(len(full)))
	info = append(info, full...)
	info = append(info, 0)
	out, _ := hkdf.Expand(sha256.New, secret, string(info), length)
	return out
}

// quicInitialCrypto removes the Initial packet protection and returns the CRYPTO frame data (the ClientHello).
func quicInitialCrypto(pkt []byte) ([]byte, error) {
	if len(pkt) < 7 || pkt[0]&0xc0 != 0xc0 || (pkt[0]>>4)&3 != 0 {
		return nil, errors.New("not a long-header Initial packet")
	}
	if binary.BigEndian.Uint32(pkt[1:5]) != 1 {
		return nil, errors.New("only QUIC version 1")
	}
	p := 5
	dcil := int(pkt[p])
	p++
	if len(pkt) < p+dcil+1 {
		return nil, errors.New("short")
	}
	dcid := pkt[p : p+dcil]
	p += dcil
	scil := int(pkt[p])
	p += 1 + scil
	tokLen, n := quicVarint(pkt[p:])
	if n == 0 {
		return nil, errors.New("short")
	}
	p += n + int(tokLen)
	length, n := quicVarint(pkt[p:])
	if n == 0 {
		return nil, errors.New("short")
	}
	p += n
	pnOff := p
	if len(pkt) < pnOff+int(length) || length < 20 {
		return nil, errors.New("short")
	}
	initial, _ := hkdf.Extract(sha256.New, dcid, quicV1Salt)
	client := hkdfExpandLabel(initial, "client in", 32)
	key := hkdfExpandLabel(client, "quic key", 16)
	iv := hkdfExpandLabel(client, "quic iv", 12)
	hp := hkdfExpandLabel(client, "quic hp", 16)

	hb, err := aes.NewCipher(hp)
	if err != nil {
		return nil, err
	}
	sample := pkt[pnOff+4 : pnOff+20]
	mask := make([]byte, 16)
	hb.Encrypt(mask, sample)
	hdr := append([]byte(nil), pkt[:pnOff+4]...)
	hdr[0] ^= mask[0] & 0x0f
	pnLen := int(hdr[0]&0x03) + 1
	var pn uint64
	for i := 0; i < pnLen; i++ {
		hdr[pnOff+i] ^= mask[1+i]
		pn = pn<<8 | uint64(hdr[pnOff+i])
	}
	hdr = hdr[:pnOff+pnLen]
	payload := pkt[pnOff+pnLen : pnOff+int(length)]
	nonce := append([]byte(nil), iv...)
	for i := 0; i < 8; i++ {
		nonce[len(nonce)-1-i] ^= byte(pn >> (8 * i))
	}
	kb, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(kb)
	plain, err := aead.Open(nil, nonce, payload, hdr)
	if err != nil {
		return nil, err
	}
	// collect CRYPTO frames (type 0x06) in offset order; skip PADDING / PING
	var crypto []byte
	type piece struct {
		off  uint64
		data []byte
	}
	var pieces []piece
	for i := 0; i < len(plain); {
		ft := plain[i]
		switch {
		case ft == 0x00 || ft == 0x01:
			i++
		case ft == 0x06:
			i++
			off, n1 := quicVarint(plain[i:])
			if n1 == 0 {
				return nil, errors.New("bad frame")
			}
			i += n1
			l, n2 := quicVarint(plain[i:])
			if n2 == 0 || i+n2+int(l) > len(plain) {
				return nil, errors.New("bad frame")
			}
			i += n2
			pieces = append(pieces, piece{off, plain[i : i+int(l)]})
			i += int(l)
		default:
			i = len(plain) // ACK or other frames: stop; the ClientHello normally comes first
		}
	}
	for want := uint64(0); ; {
		found := false
		for _, pc := range pieces {
			if pc.off == want {
				crypto = append(crypto, pc.data...)
				want += uint64(len(pc.data))
				found = true
			}
		}
		if !found {
			break
		}
	}
	return crypto, nil
}
