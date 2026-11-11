package engine

import (
	"net/netip"
	"strings"
	"syscall"
	"unsafe"
)

// systemDNSServers: the DNS servers of the PC's own adapters (for company names), read from the registry
// (HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\{adapter}: NameServer or DhcpNameServer).
func systemDNSServers() []netip.Addr {
	const base = `SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
	var root syscall.Handle
	p, _ := syscall.UTF16PtrFromString(base)
	if syscall.RegOpenKeyEx(syscall.HKEY_LOCAL_MACHINE, p, 0, syscall.KEY_READ, &root) != nil {
		return nil
	}
	defer syscall.RegCloseKey(root)
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for i := uint32(0); ; i++ {
		name := make([]uint16, 256)
		n := uint32(len(name))
		if syscall.RegEnumKeyEx(root, i, &name[0], &n, nil, nil, nil, nil) != nil {
			break
		}
		var k syscall.Handle
		if syscall.RegOpenKeyEx(root, &name[0], 0, syscall.KEY_READ, &k) != nil {
			continue
		}
		for _, v := range []string{"NameServer", "DhcpNameServer"} {
			for _, s := range strings.FieldsFunc(regString(k, v), func(r rune) bool { return r == ' ' || r == ',' }) {
				if a, err := netip.ParseAddr(s); err == nil && !seen[a] && !strings.HasPrefix(s, "198.18.") {
					seen[a] = true
					out = append(out, a)
				}
			}
		}
		syscall.RegCloseKey(k)
	}
	return out
}

func regString(k syscall.Handle, name string) string {
	np, _ := syscall.UTF16PtrFromString(name)
	var typ, size uint32
	if syscall.RegQueryValueEx(k, np, nil, &typ, nil, &size) != nil || size == 0 {
		return ""
	}
	buf := make([]uint16, size/2+1)
	if syscall.RegQueryValueEx(k, np, nil, &typ, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
