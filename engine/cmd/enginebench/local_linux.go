package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func sh(args ...string) error {
	if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %v %s", strings.Join(args, " "), err, out)
	}
	return nil
}

// setupLocal builds the private test network: a network namespace "nmbench" behind a virtual cable, with the
// servers inside, reachable from here over the cable (the engines' line).
func setupLocal() (func(), error) {
	exec.Command("ip", "rule", "del", "pref", "100").Run()
	exec.Command("ip", "netns", "del", "nmbench").Run()
	exec.Command("ip", "link", "del", "nmveth-h").Run()
	steps := [][]string{
		{"ip", "netns", "add", "nmbench"},
		{"ip", "link", "add", "nmveth-h", "type", "veth", "peer", "name", "nmveth-f"},
		{"ip", "link", "set", "nmveth-f", "netns", "nmbench"},
		{"ip", "addr", "add", "10.200.0.1/24", "dev", "nmveth-h"},
		{"ip", "link", "set", "nmveth-h", "up"},
		{"ip", "netns", "exec", "nmbench", "ip", "addr", "add", "10.200.0.2/24", "dev", "nmveth-f"},
		{"ip", "netns", "exec", "nmbench", "ip", "link", "set", "nmveth-f", "up"},
		{"ip", "netns", "exec", "nmbench", "ip", "link", "set", "lo", "up"},
		{"ip", "netns", "exec", "nmbench", "ip", "addr", "add", "203.0.113.10/32", "dev", "lo"},
		{"ip", "netns", "exec", "nmbench", "ip", "addr", "add", "203.0.113.53/32", "dev", "lo"},
		{"ip", "netns", "exec", "nmbench", "ip", "route", "add", "default", "via", "10.200.0.1"},
		// only sockets bound to the cable (the engines' line) reach the far side; everything else must be captured
		// (a route in the main table would let mihomo's routing rules skip its adapter)
		{"ip", "route", "add", "203.0.113.0/24", "via", "10.200.0.2", "dev", "nmveth-h", "table", "4242"},
		{"ip", "rule", "add", "oif", "nmveth-h", "lookup", "4242", "pref", "100"},
	}
	cleanup := func() {
		exec.Command("ip", "rule", "del", "pref", "100").Run()
		exec.Command("ip", "netns", "del", "nmbench").Run()
		exec.Command("ip", "link", "del", "nmveth-h").Run()
	}
	for _, s := range steps {
		if err := sh(s...); err != nil {
			cleanup()
			return nil, err
		}
	}
	self, _ := os.Executable()
	srv := exec.Command("ip", "netns", "exec", "nmbench", self, "-serve")
	srv.Stdout, srv.Stderr = os.Stdout, os.Stderr
	if err := srv.Start(); err != nil {
		cleanup()
		return nil, err
	}
	for i := 0; i < 100; i++ {
		d := &net.Dialer{Timeout: 200 * time.Millisecond, Control: bindTo("nmveth-h")}
		if c, err := d.Dial("tcp", "203.0.113.10:8080"); err == nil {
			c.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	tg = targets{ifName: "nmveth-h", noBaseline: true, blockCheck: true, routes: []string{"203.0.113.0/24"},
		small: "http://203.0.113.10:8080/small", dnsServer: "10.200.0.2:53", dnsControl: bindTo("nmveth-h"),
		tlsURL: "https://tls.bench.test/small", udpAddr: "203.0.113.10:5353", insecure: true,
		downloads: []dl{
			{"single", "One download, 2 GB (HTTP)", "http://dl.bench.test:8080/bytes?n=2147483648", 1, 2147483648, 1},
			{"single_tls", "One download, 1 GB (HTTPS)", "https://dl.bench.test/bytes?n=1073741824", 1, 1073741824, 1},
			{"parallel", "8 downloads at once, 8 x 512 MB (HTTP)", "http://dl.bench.test:8080/bytes?n=536870912", 8, 536870912, 1},
			{"many", "64 downloads at once, 64 x 16 MB (HTTP)", "http://dl.bench.test:8080/bytes?n=16777216", 64, 16777216, 1},
		}}
	return func() { srv.Process.Kill(); srv.Wait(); cleanup() }, nil
}

func procStats(pid int) (uint64, uint64, time.Duration, error) {
	st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, 0, 0, err
	}
	var rss, hwm uint64
	for _, l := range strings.Split(string(st), "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 {
			v, _ := strconv.ParseUint(f[1], 10, 64)
			switch f[0] {
			case "VmRSS:":
				rss = v * 1024
			case "VmHWM:":
				hwm = v * 1024
			}
		}
	}
	s, _ := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	var cpu time.Duration
	if i := strings.LastIndexByte(string(s), ')'); i > 0 {
		f := strings.Fields(string(s)[i+2:])
		if len(f) > 13 {
			u, _ := strconv.ParseInt(f[11], 10, 64)
			k, _ := strconv.ParseInt(f[12], 10, 64)
			cpu = time.Duration(u+k) * 10 * time.Millisecond
		}
	}
	return rss, hwm, cpu, nil
}

func interrupt(c *exec.Cmd)             { c.Process.Signal(syscall.SIGTERM) }
func defaultInterface() (string, error) { return "", fmt.Errorf("on Linux use -local") }

func bindTo(dev string) func(string, string, syscall.RawConn) error {
	return func(_, _ string, c syscall.RawConn) error {
		var err error
		c.Control(func(fd uintptr) { err = syscall.BindToDevice(int(fd), dev) })
		return err
	}
}
