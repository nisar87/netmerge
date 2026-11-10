//go:build !windows

package main

// Development stubs so the UI can be run and tested on Linux with fake data.

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func listAdapters() ([]Adapter, error) {
	if os.Getenv("NM_STUB_LOOPBACK") != "" { // development only: two lines on the loopback, for testing end to end
		return []Adapter{
			{Name: "Ethernet", Index: 11, Desc: "Intel(R) Ethernet Connection (2) I219-V", IP: "127.0.0.1", Up: true},
			{Name: "Wi-Fi", Index: 7, Desc: "TP-Link Wireless USB Adapter", IP: "127.0.0.2", Up: true},
		}, nil
	}
	switch os.Getenv("NM_STUB_ADAPTERS") { // development only: other combinations of connections
	case "twowifi":
		return []Adapter{
			{Name: "Wi-Fi", Index: 7, Desc: "Intel(R) Wi-Fi 6 AX201 160MHz", IP: "192.168.1.3", Gateway: "192.168.1.1", LinkRate: "866 Mbps", Up: true},
			{Name: "Wi-Fi 2", Index: 9, Desc: "TP-Link Wireless USB Adapter", IP: "192.168.1.4", Gateway: "192.168.1.1", LinkRate: "300 Mbps", Up: true},
		}, nil
	case "four":
		return []Adapter{
			{Name: "Ethernet", Index: 11, Desc: "Intel(R) Ethernet Connection (2) I219-V", IP: "192.168.1.110", Gateway: "192.168.1.1", LinkRate: "1 Gbps", Up: true},
			{Name: "Wi-Fi", Index: 7, Desc: "Intel(R) Wi-Fi 6 AX201 160MHz", IP: "192.168.1.3", Gateway: "192.168.1.1", LinkRate: "866 Mbps", Up: true},
			{Name: "Wi-Fi 2", Index: 9, Desc: "TP-Link Wireless USB Adapter", IP: "192.168.1.4", Gateway: "192.168.1.1", LinkRate: "300 Mbps", Up: true},
			{Name: "Ethernet 3", Index: 14, Desc: "Remote NDIS based Internet Sharing Device", IP: "192.168.42.2", Gateway: "192.168.42.129", LinkRate: "100 Mbps", Up: true},
		}, nil
	}
	return []Adapter{
		{Name: "Ethernet", Index: 11, Desc: "Intel(R) Ethernet Connection (2) I219-V", IP: "192.168.1.110", Gateway: "192.168.1.1", LinkRate: "1 Gbps", Up: true},
		{Name: "Wi-Fi", Index: 7, Desc: "TP-Link Wireless USB Adapter", IP: "192.168.1.3", Gateway: "192.168.0.1", LinkRate: "300 Mbps", Up: true},
		{Name: "Ethernet 2", Index: 15, Desc: "SonicWALL Virtual NIC", IP: "10.9.0.4", Up: true},
	}, nil
}

var fakeStart = time.Now()
var fakeTot = map[int][2]uint32{}

func ifCounters() map[int][2]uint32 {
	t := time.Since(fakeStart).Seconds()
	for idx, base := range map[int]float64{11: 3.2e6, 7: 7.4e6, 9: 4.1e6, 14: 1.2e6} {
		v := fakeTot[idx]
		rx := base * (0.55 + 0.4*math.Abs(math.Sin(t/7+float64(idx)))) * (0.8 + rand.Float64()*0.3) / 2
		v[0] += uint32(rx)
		v[1] += uint32(rx / 9)
		fakeTot[idx] = v
	}
	return fakeTot
}

func bindControl(ifIndex int) func(network, address string, c syscall.RawConn) error { return nil }
func isAdmin() bool                                                                  { return true }
func relaunchAsAdmin() bool                                                          { return false }
func messageBox(title, text string)                                                  { logf("%s: %s", title, text) }
func openWindow(url string)                                                          { logf("open %s", url) }
func killProcessByName(name string)                                                  {}
func setAutoStart(on bool) error                                                     { return nil }
func createShortcut()                                                                {}
func hiddenCmd(name string, args ...string) *exec.Cmd                                { return exec.Command(name, args...) }
func startEngineProcess(exe string, args ...string) (*exec.Cmd, error) {
	c := exec.Command(exe, args...)
	return c, c.Start()
}

func setShareFirewall(on bool, port, ssPort int) {}

func publicNetworks() []string { return nil }

func startUpdater(newExe, curExe, prevExe string) error {
	return fmt.Errorf("updating is only available on Windows")
}
