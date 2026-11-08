package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

// Name lookups for downloads. When NetMerge's own DNS is on, the PC's normal lookups are answered with made-up
// addresses (198.18.x.x) that only mean something inside the merge tunnel. Downloads leave through a chosen
// line without the tunnel, so such an address leads nowhere: the name is asked again out of that very line.

var (
	dlLookup  = func(ctx context.Context, host string) ([]net.IP, error) { return dlSystemLookup(ctx, host) }
	dlDNSPort = "53"

	dlResMu    sync.Mutex
	dlResCache = map[string]dlResEntry{}
)

type dlResEntry struct {
	ips []net.IP
	at  time.Time
}

func dlSystemLookup(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	var out []net.IP
	for _, a := range addrs {
		if a.IP.To4() != nil {
			out = append(out, a.IP.To4())
		}
	}
	if len(out) == 0 && err == nil {
		err = errors.New("no IPv4 address")
	}
	return out, err
}

// dlFakeIP: addresses of the benchmarking range 198.18.0.0/15, used by the merge engine's fake DNS answers.
func dlFakeIP(ip net.IP) bool {
	v := ip.To4()
	return v != nil && v[0] == 198 && (v[1] == 18 || v[1] == 19)
}

func dlRealOnly(ips []net.IP) []net.IP {
	var out []net.IP
	for _, ip := range ips {
		if !dlFakeIP(ip) {
			out = append(out, ip)
		}
	}
	return out
}

// dlResolveOn gives the IPv4 addresses of host as seen from the adapter a.
func dlResolveOn(ctx context.Context, a Adapter, host string) ([]net.IP, error) {
	key := a.Name + "|" + host
	dlResMu.Lock()
	if e, ok := dlResCache[key]; ok && time.Since(e.at) < 5*time.Minute {
		dlResMu.Unlock()
		return e.ips, nil
	}
	dlResMu.Unlock()
	store := func(ips []net.IP) ([]net.IP, error) {
		dlResMu.Lock()
		if len(dlResCache) > 2000 {
			dlResCache = map[string]dlResEntry{}
		}
		dlResCache[key] = dlResEntry{ips, time.Now()}
		dlResMu.Unlock()
		return ips, nil
	}
	sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	ips, err := dlLookup(sctx, host)
	cancel()
	if err == nil && len(ips) > 0 {
		if real := dlRealOnly(ips); len(real) > 0 {
			return store(real)
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	// ask out of the line itself: its router first, then two public servers
	var servers []string
	if a.Gateway != "" {
		servers = append(servers, a.Gateway)
	}
	servers = append(servers, "1.1.1.1", "8.8.8.8")
	lastErr := err
	for _, srv := range servers {
		srv := srv
		r := &net.Resolver{PreferGo: true, Dial: func(c context.Context, network, _ string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 3 * time.Second, Control: bindControl(a.Index)}
			if ip := net.ParseIP(a.IP); ip != nil && ip.To4() != nil {
				if network == "tcp" || network == "tcp4" {
					d.LocalAddr = &net.TCPAddr{IP: ip}
				} else {
					d.LocalAddr = &net.UDPAddr{IP: ip}
				}
			}
			return d.DialContext(c, network, net.JoinHostPort(srv, dlDNSPort))
		}}
		qctx, qc := context.WithTimeout(ctx, 4*time.Second)
		addrs, e := r.LookupIP(qctx, "ip4", host)
		qc()
		if e != nil {
			lastErr = e
			continue
		}
		if real := dlRealOnly(addrs); len(real) > 0 {
			return store(real)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("the name could not be found")
	}
	return nil, lastErr
}

// dlDialOn opens a connection out of the adapter a, looking the name up through that adapter.
func dlDialOn(ctx context.Context, a Adapter, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: 12 * time.Second, KeepAlive: 30 * time.Second, Control: bindControl(a.Index)}
	if ip := net.ParseIP(a.IP); ip != nil && ip.To4() != nil {
		d.LocalAddr = &net.TCPAddr{IP: ip}
	}
	if network == "tcp" {
		network = "tcp4"
	}
	if net.ParseIP(host) != nil {
		return d.DialContext(ctx, network, addr)
	}
	ips, err := dlResolveOn(ctx, a, host)
	if err != nil {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	var last error
	for _, ip := range ips {
		c, e := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if e == nil {
			return c, nil
		}
		last = e
		if ctx.Err() != nil {
			break
		}
	}
	return nil, last
}
