package engine

import (
	"net"
	"testing"
	"time"
)

// The Windows VPN packet path: only the socket that sent the right hello is listened to, and replies go to it.
func TestLoopDevAcceptsOnlyThePlugIn(t *testing.T) {
	if _, err := openLoopDev(0, "short"); err == nil {
		t.Fatal("a missing port or short token must be refused")
	}
	probe, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	token := "0123456789abcdef0123"
	d, err := openLoopDev(port, token)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	dial := func() *net.UDPConn {
		c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	plugin, intruder := dial(), dial()
	defer plugin.Close()
	defer intruder.Close()

	got := make(chan []byte, 4)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, err := d.Read(buf)
			if err != nil {
				return
			}
			got <- append([]byte(nil), buf[:n]...)
		}
	}()
	pkt := []byte{0x45, 0, 0, 20, 1, 2, 3, 4}
	// before any hello nothing is accepted, and nothing can be sent
	intruder.Write(pkt)
	if err := d.Write(pkt); err != nil {
		t.Fatal(err)
	}
	intruder.Write([]byte(vpnHelloPrefix + "wrong-token-xxxxxxx"))
	plugin.Write([]byte(vpnHelloPrefix + token))
	select {
	case <-d.seen:
	case <-time.After(2 * time.Second):
		t.Fatal("the plug-in's hello was not accepted")
	}
	intruder.Write(pkt)
	plugin.Write(pkt)
	select {
	case b := <-got:
		if string(b) != string(pkt) {
			t.Fatalf("got %x", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the plug-in's packet did not arrive")
	}
	select {
	case b := <-got:
		t.Fatalf("a second packet arrived (the intruder's?): %x", b)
	case <-time.After(200 * time.Millisecond):
	}
	// replies go to the plug-in
	if err := d.Write([]byte{0x45, 9, 9}); err != nil {
		t.Fatal(err)
	}
	plugin.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := plugin.Read(buf)
	if err != nil || string(buf[:n]) != string([]byte{0x45, 9, 9}) {
		t.Fatalf("reply: %x %v", buf[:n], err)
	}
}
