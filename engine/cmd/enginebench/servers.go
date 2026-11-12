package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"time"
)

// runServers is the far side of the private test network: web (HTTP 8080, HTTPS 443) and UDP echo on
// 203.0.113.10, and a DNS server (203.0.113.53) that answers 203.0.113.10 for every name.
func runServers() {
	mux := http.NewServeMux()
	mux.HandleFunc("/small", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hello from the far side\n")) })
	zeros := make([]byte, 256<<10)
	mux.HandleFunc("/bytes", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.ParseInt(r.URL.Query().Get("n"), 10, 64)
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		for n > 0 {
			k := int64(len(zeros))
			if n < k {
				k = n
			}
			if _, err := w.Write(zeros[:k]); err != nil {
				return
			}
			n -= k
		}
	})
	go http.ListenAndServe("203.0.113.10:8080", mux)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "bench.test"},
		DNSNames: []string{"*.bench.test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	srv := &http.Server{Addr: "203.0.113.10:443", Handler: mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}}
	go srv.ListenAndServeTLS("", "")
	go func() {
		c, err := net.ListenPacket("udp", "203.0.113.10:5353")
		if err != nil {
			return
		}
		b := make([]byte, 65535)
		for {
			n, a, err := c.ReadFrom(b)
			if err != nil {
				return
			}
			c.WriteTo(b[:n], a)
		}
	}()
	go dnsServer("10.200.0.2:53")
	dnsServer("203.0.113.53:53")
}

// dnsServer answers from the address it was asked at (one socket per address)
func dnsServer(addr string) {
	c, err := net.ListenPacket("udp", addr)
	if err != nil {
		panic(err)
	}
	b := make([]byte, 1500)
	for {
		n, a, err := c.ReadFrom(b)
		if err != nil || n < 17 {
			continue
		}
		q := b[:n]
		i := 12
		for i < n && q[i] != 0 {
			i += 1 + int(q[i])
		}
		qe := i + 5
		if qe > n {
			continue
		}
		qt := binary.BigEndian.Uint16(q[i+1:])
		r := append([]byte{}, q[:2]...)
		r = append(r, 0x81, 0x80, 0, 1)
		if qt == 1 {
			r = append(r, 0, 1, 0, 0, 0, 0)
			r = append(r, q[12:qe]...)
			r = append(r, 0xc0, 12, 0, 1, 0, 1, 0, 0, 1, 0x2c, 0, 4, 203, 0, 113, 10)
		} else {
			r = append(r, 0, 0, 0, 0, 0, 0)
			r = append(r, q[12:qe]...)
		}
		c.WriteTo(r, a)
	}
}
