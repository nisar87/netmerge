package main

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
)

// a tiny DNS server: answers every A question with one fixed address
func fakeDNS(t *testing.T, answer string) (port string, queries *int32) {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	var n int32
	go func() {
		buf := make([]byte, 512)
		for {
			k, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if k < 13 {
				continue
			}
			atomic.AddInt32(&n, 1)
			q := append([]byte{}, buf[:k]...)
			// end of the question: name labels, 0, type, class
			i := 12
			for i < k && q[i] != 0 {
				i += int(q[i]) + 1
			}
			end := i + 1 + 4
			if end > k {
				continue
			}
			resp := append([]byte{}, q[:end]...)
			resp[2], resp[3] = 0x81, 0x80
			resp[6], resp[7] = 0, 1 // one answer
			resp[8], resp[9], resp[10], resp[11] = 0, 0, 0, 0
			ip := net.ParseIP(answer).To4()
			resp = append(resp, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, ip[0], ip[1], ip[2], ip[3])
			pc.WriteTo(resp, from)
		}
	}()
	_, p, _ := net.SplitHostPort(pc.LocalAddr().String())
	return p, &n
}

func withLookup(t *testing.T, f func(host string) ([]net.IP, error)) {
	old, oldPort := dlLookup, dlDNSPort
	dlLookup = func(_ context.Context, h string) ([]net.IP, error) { return f(h) }
	dlResMu.Lock()
	dlResCache = map[string]dlResEntry{}
	dlResMu.Unlock()
	t.Cleanup(func() { dlLookup, dlDNSPort = old, oldPort })
}

func TestFakeDNSAnswersAreNotUsedForDownloads(t *testing.T) {
	port, q := fakeDNS(t, "203.0.113.9")
	withLookup(t, func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("198.18.0.77")}, nil })
	dlDNSPort = port
	a := Adapter{Name: "Wi-Fi 2", IP: "127.0.0.1", Gateway: "127.0.0.1", Index: 9}
	ips, err := dlResolveOn(context.Background(), a, "files.example.test")
	if err != nil || len(ips) != 1 || ips[0].String() != "203.0.113.9" {
		t.Fatalf("%v %v", ips, err)
	}
	if atomic.LoadInt32(q) == 0 {
		t.Fatal("the line's own DNS was not asked")
	}
	// the answer is remembered
	before := atomic.LoadInt32(q)
	if _, err := dlResolveOn(context.Background(), a, "files.example.test"); err != nil || atomic.LoadInt32(q) != before {
		t.Fatal("no cache")
	}
	// the cache is per line
	b := Adapter{Name: "Ethernet", IP: "127.0.0.1", Gateway: "127.0.0.1", Index: 11}
	if _, err := dlResolveOn(context.Background(), b, "files.example.test"); err != nil || atomic.LoadInt32(q) == before {
		t.Fatal("two lines shared a cache entry")
	}
}

func TestRealDNSAnswersAreUsedAsTheyAre(t *testing.T) {
	port, q := fakeDNS(t, "203.0.113.9")
	withLookup(t, func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil })
	dlDNSPort = port
	a := Adapter{Name: "Wi-Fi", IP: "127.0.0.1", Gateway: "127.0.0.1", Index: 7}
	ips, err := dlResolveOn(context.Background(), a, "example.com")
	if err != nil || ips[0].String() != "93.184.216.34" || atomic.LoadInt32(q) != 0 {
		t.Fatalf("%v %v queries=%d", ips, err, atomic.LoadInt32(q))
	}
}

func TestFailingSystemDNSFallsBackToTheLine(t *testing.T) {
	port, _ := fakeDNS(t, "203.0.113.10")
	withLookup(t, func(string) ([]net.IP, error) { return nil, context.DeadlineExceeded })
	dlDNSPort = port
	a := Adapter{Name: "Wi-Fi", IP: "127.0.0.1", Gateway: "127.0.0.1", Index: 7}
	ips, err := dlResolveOn(context.Background(), a, "example.org")
	if err != nil || ips[0].String() != "203.0.113.10" {
		t.Fatalf("%v %v", ips, err)
	}
}

func TestDialOnUsesTheResolvedAddress(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, p, _ := net.SplitHostPort(ln.Addr().String())
	withLookup(t, func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil })
	a := Adapter{Name: "Wi-Fi", IP: "127.0.0.1", Index: 7}
	c, err := dlDialOn(context.Background(), a, "tcp", "somewhere.example:"+p)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	// a name that nobody knows: an error the person can understand
	withLookup(t, func(string) ([]net.IP, error) { return nil, &net.DNSError{Err: "no such host", IsNotFound: true} })
	dlDNSPort = "1" // nothing listens there
	if _, err := dlDialOn(context.Background(), Adapter{Name: "X", IP: "127.0.0.1", Gateway: "127.0.0.1"}, "tcp", "nope.example:80"); err == nil {
		t.Fatal("expected an error")
	} else if msg, _ := dlFriendly(err); msg == "" || msg == err.Error() && len(msg) > 120 {
		t.Fatalf("unfriendly: %q", msg)
	}
}
