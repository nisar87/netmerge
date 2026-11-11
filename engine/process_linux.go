package engine

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Linux (tests and development): the socket's inode from /proc/net/*, then the process holding it.

func lookupOwner(network string, ip netip.Addr, port uint16) (uint32, bool) {
	files := []string{"/proc/net/tcp", "/proc/net/tcp6"}
	if network == "udp" {
		files = []string{"/proc/net/udp", "/proc/net/udp6"}
	}
	want := fmt.Sprintf(":%04X", port)
	inode := ""
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Scan()
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 10 || !strings.HasSuffix(fs[1], want) {
				continue
			}
			if a := procHexAddr(strings.TrimSuffix(fs[1], want)); a.IsValid() && !a.IsUnspecified() && a.Unmap() != ip.Unmap() {
				continue
			}
			inode = fs[9]
			break
		}
		fh.Close()
		if inode != "" {
			break
		}
	}
	if inode == "" || inode == "0" {
		return 0, false
	}
	target := "socket:[" + inode + "]"
	procs, _ := os.ReadDir("/proc")
	for _, d := range procs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", d.Name(), "fd"))
		for _, fd := range fds {
			if l, err := os.Readlink(filepath.Join("/proc", d.Name(), "fd", fd.Name())); err == nil && l == target {
				return uint32(pid), true
			}
		}
	}
	return 0, false
}

func procHexAddr(h string) netip.Addr {
	b, err := hex.DecodeString(h)
	if err != nil {
		return netip.Addr{}
	}
	// /proc stores each 32-bit word in host (little-endian) order
	for i := 0; i+4 <= len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

func processPath(pid uint32) string {
	p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		return strings.TrimSpace(string(b))
	}
	return p
}
