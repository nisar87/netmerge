package engine

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Windows: the virtual adapter is Wintun (wintun.dll, shipped inside NetMerge.exe). Addresses, routes, DNS and the
// firewall permission are set with netsh; every route lives on the adapter itself (so it disappears with it),
// except the "keep local addresses out" routes, which point at the real default gateway and are removed on stop.

//go:embed wintun
var wintunFS embed.FS

// wintunSHA256 is the fingerprint of wintun.dll (amd64) from wintun-0.14.1.zip, published by the WireGuard project.
// The build puts the file in engine/wintun/amd64/; the engine refuses any other file.
var wintunSHA256 = ""

var (
	wintunOnce                                                                                     sync.Once
	wintunErr                                                                                      error
	wtCreate, wtOpen, wtClose, wtLUID, wtStart, wtEnd, wtEvent, wtRecv, wtRelease, wtAlloc, wtSend *syscall.Proc

	procConvertLuidToIndex = modIphlpapi.NewProc("ConvertInterfaceLuidToIndex")
	procGetIpForwardTable  = modIphlpapi.NewProc("GetIpForwardTable")
	procWaitForSingle      = modKernel32.NewProc("WaitForSingleObject")
)

func loadWintun(dir string) error {
	wintunOnce.Do(func() {
		data, err := wintunFS.ReadFile("wintun/amd64/wintun.dll")
		if err != nil {
			wintunErr = errors.New("this NetMerge build does not include the virtual adapter driver (wintun.dll)")
			return
		}
		sum := sha256.Sum256(data)
		if wintunSHA256 != "" && hex.EncodeToString(sum[:]) != wintunSHA256 {
			wintunErr = errors.New("wintun.dll in this build has an unexpected fingerprint")
			return
		}
		path := filepath.Join(dir, "wintun.dll")
		if old, err := os.ReadFile(path); err != nil || !bytes.Equal(old, data) {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				wintunErr = fmt.Errorf("cannot write wintun.dll: %w", err)
				return
			}
		}
		dll, err := syscall.LoadDLL(path)
		if err != nil {
			wintunErr = fmt.Errorf("cannot load wintun.dll: %w", err)
			return
		}
		get := func(n string) *syscall.Proc {
			p, err := dll.FindProc(n)
			if err != nil && wintunErr == nil {
				wintunErr = err
			}
			return p
		}
		wtCreate, wtOpen, wtClose = get("WintunCreateAdapter"), get("WintunOpenAdapter"), get("WintunCloseAdapter")
		wtLUID, wtStart, wtEnd = get("WintunGetAdapterLUID"), get("WintunStartSession"), get("WintunEndSession")
		wtEvent, wtRecv, wtRelease = get("WintunGetReadWaitEvent"), get("WintunReceivePacket"), get("WintunReleaseReceivePacket")
		wtAlloc, wtSend = get("WintunAllocateSendPacket"), get("WintunSendPacket")
	})
	return wintunErr
}

// the same adapter identity every time, so Windows does not collect a new network profile per start
var netmergeGUID = [16]byte{0x5c, 0x3a, 0x1f, 0x7e, 0x9b, 0x42, 0x4d, 0x1e, 0x8a, 0x6b, 0x2f, 0x11, 0xc0, 0x9d, 0x3e, 0x71}

type wintunDev struct {
	adapter uintptr
	session uintptr
	event   uintptr
	name    string
	index   int
	closed  bool
	mu      sync.Mutex
}

func openTun(name string, mtu int) (tunDevice, error) {
	exe, _ := os.Executable()
	if err := loadWintun(filepath.Dir(exe)); err != nil {
		return nil, err
	}
	n16, _ := syscall.UTF16PtrFromString(name)
	t16, _ := syscall.UTF16PtrFromString("NetMerge")
	// a leftover adapter (after a crash) still has routes pointing at it: remove it first
	if h, _, _ := wtOpen.Call(uintptr(unsafe.Pointer(n16))); h != 0 {
		wtClose.Call(h)
		time.Sleep(300 * time.Millisecond)
	}
	h, _, err := wtCreate.Call(uintptr(unsafe.Pointer(n16)), uintptr(unsafe.Pointer(t16)), uintptr(unsafe.Pointer(&netmergeGUID[0])))
	if h == 0 {
		return nil, fmt.Errorf("cannot create the virtual adapter: %v", err)
	}
	var luid uint64
	wtLUID.Call(h, uintptr(unsafe.Pointer(&luid)))
	var idx uint32
	if r, _, _ := procConvertLuidToIndex.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&idx))); r != 0 {
		wtClose.Call(h)
		return nil, fmt.Errorf("cannot find the virtual adapter's number (%d)", r)
	}
	s, _, err := wtStart.Call(h, 0x800000) // 8 MiB ring
	if s == 0 {
		wtClose.Call(h)
		return nil, fmt.Errorf("cannot start the virtual adapter: %v", err)
	}
	ev, _, _ := wtEvent.Call(s)
	return &wintunDev{adapter: h, session: s, event: ev, name: name, index: int(idx)}, nil
}

func (d *wintunDev) Name() string { return d.name }

func (d *wintunDev) Read(b []byte) (int, error) {
	for {
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return 0, net.ErrClosed
		}
		var size uint32
		p, _, err := wtRecv.Call(d.session, uintptr(unsafe.Pointer(&size)))
		if p != 0 {
			n := copy(b, unsafe.Slice((*byte)(unsafe.Add(nil, p)), size)) // memory owned by wintun until released
			wtRelease.Call(d.session, p)
			d.mu.Unlock()
			return n, nil
		}
		d.mu.Unlock()
		if errno, ok := err.(syscall.Errno); ok && errno == 259 { // ERROR_NO_MORE_ITEMS: wait for packets
			procWaitForSingle.Call(d.event, 250)
			continue
		}
		if errno, ok := err.(syscall.Errno); ok && errno == 38 { // ERROR_HANDLE_EOF: the adapter went away
			return 0, net.ErrClosed
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (d *wintunDev) Write(pkt []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return net.ErrClosed
	}
	p, _, err := wtAlloc.Call(d.session, uintptr(len(pkt)))
	if p == 0 {
		return err
	}
	copy(unsafe.Slice((*byte)(unsafe.Add(nil, p)), len(pkt)), pkt)
	wtSend.Call(d.session, p)
	return nil
}

func (d *wintunDev) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	wtEnd.Call(d.session)
	wtClose.Call(d.adapter) // removes the adapter and every route on it
	return nil
}

// ---- addresses, routes, DNS, firewall ----

type winState struct {
	idx      int
	excludes []string // prefixes kept out of the capture
	exGW     netip.Addr
	exIdx    int
	mu       sync.Mutex
	stop     chan struct{}
}

func netsh(args ...string) error {
	cmd := exec.Command("netsh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (t *tunStack) setupOS(c *Config) error {
	d := t.dev.(*wintunDev)
	ws := &winState{idx: d.index, stop: make(chan struct{})}
	t.osState = ws
	idx := itoa(d.index)
	steps := [][]string{
		{"interface", "ipv4", "set", "address", "name=" + idx, "source=static", "address=" + tunGW4.String(), "mask=255.255.0.0"},
		{"interface", "ipv4", "set", "subinterface", idx, "mtu=" + itoa(t.mtu), "store=active"},
		{"interface", "ipv4", "set", "interface", idx, "metric=1"},
	}
	for _, s := range steps {
		if err := netsh(s...); err != nil {
			return err
		}
	}
	// IPv6 too (best effort: some PCs have IPv6 switched off)
	if err := netsh("interface", "ipv6", "add", "address", "interface="+idx, "address="+tunGW6.String()+"/126", "store=active"); err == nil {
		netsh("interface", "ipv6", "set", "subinterface", idx, "mtu="+itoa(max(t.mtu, 1280)), "store=active")
		netsh("interface", "ipv6", "set", "interface", idx, "metric=1")
	} else {
		t.e.log.Warnf("[TUN] IPv6 address: %v", err)
	}
	// wait until Windows really has the address (the listener binds to it)
	for i := 0; i < 50; i++ {
		if c, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: tunGW4.AsSlice()}); err == nil {
			c.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	// the firewall must let rewritten connections in from the adapter
	if exe, err := os.Executable(); err == nil {
		netsh("advfirewall", "firewall", "delete", "rule", "name=NetMerge engine")
		if err := netsh("advfirewall", "firewall", "add", "rule", "name=NetMerge engine", "dir=in", "action=allow", "program="+exe, "enable=yes", "profile=any"); err != nil {
			t.e.log.Warnf("[TUN] firewall: %v", err)
		}
	}
	if t.hijack.Load() {
		netsh("interface", "ipv4", "set", "dnsservers", "name="+idx, "source=static", "address="+tunDNS4.String(), "register=none", "validate=no")
	}
	// the PC's default line, for DIRECT and for keeping local addresses out
	t.watchOS()
	if c.Tun.AutoRoute {
		routes4 := c.Tun.RouteAddress
		if len(routes4) == 0 {
			routes4 = []string{"0.0.0.0/1", "128.0.0.0/1", "::/1", "8000::/1"}
		}
		for _, r := range routes4 {
			fam, hop := "ipv4", "0.0.0.0"
			if strings.Contains(r, ":") {
				fam, hop = "ipv6", "::"
			}
			if err := netsh("interface", fam, "add", "route", "prefix="+r, "interface="+idx, "nexthop="+hop, "metric=1", "store=active"); err != nil && fam == "ipv4" {
				return err
			}
		}
		t.updateExcludes(c.Tun.RouteExcludeAddress)
	}
	go t.watchLoop(ws)
	return nil
}

func (t *tunStack) teardownOS() {
	ws, _ := t.osState.(*winState)
	if ws == nil {
		return
	}
	close(ws.stop)
	ws.mu.Lock()
	ex, gw, ix := ws.excludes, ws.exGW, ws.exIdx
	ws.excludes = nil
	ws.mu.Unlock()
	for _, p := range ex {
		netsh("interface", "ipv4", "delete", "route", "prefix="+p, "interface="+itoa(ix), "nexthop="+gw.String(), "store=active")
	}
}

// updateExcludes keeps the given IPv4 ranges on the real default gateway instead of the virtual adapter.
func (t *tunStack) updateExcludes(list []string) {
	ws, _ := t.osState.(*winState)
	if ws == nil {
		return
	}
	_, gw, ix := defaultRoute(ws.idx)
	ws.mu.Lock()
	old, oldGW, oldIdx := ws.excludes, ws.exGW, ws.exIdx
	ws.mu.Unlock()
	same := gw == oldGW && ix == oldIdx && strings.Join(old, ",") == strings.Join(list, ",")
	if same {
		return
	}
	for _, p := range old {
		netsh("interface", "ipv4", "delete", "route", "prefix="+p, "interface="+itoa(oldIdx), "nexthop="+oldGW.String(), "store=active")
	}
	var now []string
	if gw.IsValid() && ix > 0 {
		for _, p := range list {
			if strings.Contains(p, ":") {
				continue
			}
			if !strings.Contains(p, "/") {
				p += "/32"
			}
			if netsh("interface", "ipv4", "add", "route", "prefix="+p, "interface="+itoa(ix), "nexthop="+gw.String(), "metric=1", "store=active") == nil {
				now = append(now, p)
			}
		}
	}
	ws.mu.Lock()
	ws.excludes, ws.exGW, ws.exIdx = now, gw, ix
	ws.mu.Unlock()
}

// watchOS notices when the PC's default line changes (Wi-Fi joined, cable unplugged).
func (t *tunStack) watchOS() {
	ws, _ := t.osState.(*winState)
	if ws == nil {
		return
	}
	name, _, _ := defaultRoute(ws.idx)
	if name != t.e.defaultInterface() {
		if name != "" {
			t.e.log.Warnf("[TUN] default interface changed by monitor, => %s", name)
		}
		t.e.defIface.Store(name)
		if c := t.e.config(); c != nil && c.Tun.AutoRoute {
			t.updateExcludes(c.Tun.RouteExcludeAddress)
		}
	}
}

func (t *tunStack) watchLoop(ws *winState) {
	tk := time.NewTicker(3 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-ws.stop:
			return
		case <-tk.C:
			t.watchOS()
		}
	}
}

// defaultRoute finds the IPv4 default route with the lowest metric that is not on the virtual adapter.
func defaultRoute(tunIdx int) (string, netip.Addr, int) {
	size := uint32(32 * 1024)
	var buf []byte
	for i := 0; i < 4; i++ {
		buf = make([]byte, size)
		r, _, _ := procGetIpForwardTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 1)
		if r == 0 {
			break
		}
		if r != 122 {
			return "", netip.Addr{}, 0
		}
		buf = nil
	}
	if len(buf) < 4 {
		return "", netip.Addr{}, 0
	}
	n := int(binary.LittleEndian.Uint32(buf))
	best, bestMetric := -1, uint32(1<<31)
	var gw netip.Addr
	for i := 0; i < n && 4+(i+1)*56 <= len(buf); i++ {
		row := buf[4+i*56:]
		dest, mask := binary.LittleEndian.Uint32(row[0:]), binary.LittleEndian.Uint32(row[4:])
		idx := int(binary.LittleEndian.Uint32(row[16:]))
		metric := binary.LittleEndian.Uint32(row[36:])
		if dest != 0 || mask != 0 || idx == tunIdx {
			continue
		}
		if metric < bestMetric {
			best, bestMetric = idx, metric
			gw = netip.AddrFrom4([4]byte(row[12:16]))
		}
	}
	if best < 0 {
		return "", netip.Addr{}, 0
	}
	ifc, err := net.InterfaceByIndex(best)
	if err != nil {
		return "", netip.Addr{}, 0
	}
	return ifc.Name, gw, best
}
