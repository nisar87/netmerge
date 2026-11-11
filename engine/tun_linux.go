package engine

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

// Linux: used to test the capture path end to end (NetMerge itself runs on Windows). The adapter is a /dev/net/tun
// device; addresses and routes are set with the ip tool.

type linuxTun struct {
	f    *os.File
	name string
}

func openTun(name string, mtu int) (tunDevice, error) {
	name = "nmtun"
	// attach the device first, then hand it to Go's poller (registering it before TUNSETIFF gets no wake-ups)
	fd, err := syscall.Open("/dev/net/tun", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var req [40]byte
	copy(req[:16], name)
	*(*uint16)(unsafe.Pointer(&req[16])) = 0x0001 | 0x1000 // IFF_TUN | IFF_NO_PI
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x400454ca, uintptr(unsafe.Pointer(&req[0]))); e != 0 {
		syscall.Close(fd)
		return nil, fmt.Errorf("TUNSETIFF: %v", e)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &linuxTun{f: os.NewFile(uintptr(fd), "/dev/net/tun"), name: name}, nil
}

func (t *linuxTun) Name() string               { return t.name }
func (t *linuxTun) Read(b []byte) (int, error) { return t.f.Read(b) }
func (t *linuxTun) Write(p []byte) error       { _, err := t.f.Write(p); return err }
func (t *linuxTun) Close() error               { return t.f.Close() }

func ipCmd(args ...string) error {
	out, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %v %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (t *tunStack) setupOS(c *Config) error {
	n := t.dev.Name()
	if err := ipCmd("addr", "add", tunGW4.String()+"/16", "dev", n); err != nil {
		return err
	}
	_ = ipCmd("-6", "addr", "add", tunGW6.String()+"/126", "dev", n, "nodad")
	if err := ipCmd("link", "set", n, "up", "mtu", itoa(t.mtu)); err != nil {
		return err
	}
	t.watchOS()
	if c.Tun.AutoRoute {
		routes := c.Tun.RouteAddress
		if len(routes) == 0 {
			routes = []string{"0.0.0.0/1", "128.0.0.0/1"}
		}
		for _, r := range routes {
			args := []string{"route", "add", r, "dev", n}
			if strings.Contains(r, ":") {
				args = append([]string{"-6"}, args...)
			}
			if err := ipCmd(args...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *tunStack) teardownOS()                  {} // the device's routes go with it
func (t *tunStack) updateExcludes(list []string) {}

func (t *tunStack) watchOS() {
	out, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return
	}
	f := strings.Fields(string(out))
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "dev" && f[i+1] != t.dev.Name() {
			t.e.defIface.Store(f[i+1])
			return
		}
	}
}
