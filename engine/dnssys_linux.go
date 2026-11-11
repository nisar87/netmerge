package engine

import (
	"net/netip"
	"os"
	"strings"
)

// systemDNSServers: the PC's own DNS servers (for company names), from /etc/resolv.conf.
func systemDNSServers() []netip.Addr {
	b, _ := os.ReadFile("/etc/resolv.conf")
	var out []netip.Addr
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "nameserver" {
			if a, err := netip.ParseAddr(f[1]); err == nil {
				out = append(out, a)
			}
		}
	}
	return out
}
