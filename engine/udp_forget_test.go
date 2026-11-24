package engine

import (
	"net"
	"net/netip"
	"testing"
)

// Closing a UDP flow through the API (what NetMerge does after an app's rule changes) forgets it, so the next
// packet goes through the rules again; the open-connection list no longer shows it.
func TestClosingAUDPFlowForgetsIt(t *testing.T) {
	e := &Engine{tracker: newTracker(), stopped: make(chan struct{})}
	defer close(e.stopped)
	u := newUDPTable(e)
	remote := netip.MustParseAddrPort("198.51.100.7:3478")
	sess := &udpSession{key: "s", flows: map[netip.AddrPort]*udpFlow{}}
	f := &udpFlow{key: "10.0.0.2:5000>198.51.100.7:3478", sess: sess, remote: remote}
	u.mu.Lock()
	u.flows[f.key] = f
	u.mu.Unlock()
	sess.flows[remote] = f
	m := &Metadata{Network: "udp", Process: "ms-teams.exe", Host: "teams.example"}
	f.tr = e.tracker.add(m, []string{"LINK2"}, nil, udpForget{u, f})
	if n := len(e.tracker.snapshot()["connections"].([]map[string]any)); n != 1 {
		t.Fatalf("listed before closing: %d", n)
	}
	if !e.tracker.closeID(f.tr.id) {
		t.Fatal("closeID did not find the flow")
	}
	u.mu.Lock()
	_, still := u.flows[f.key]
	u.mu.Unlock()
	if still || sess.flows[remote] != nil {
		t.Fatal("the flow is still remembered after closing")
	}
	if n := len(e.tracker.snapshot()["connections"].([]map[string]any)); n != 0 {
		t.Fatalf("still listed after closing: %d", n)
	}
}

// A flow whose line can no longer send (its adapter went away or lost its network) is forgotten after a few failed
// sends, so an app that keeps retrying (a VPN that is "connecting") is routed again instead of failing forever.
func TestAFlowThatCannotSendIsForgotten(t *testing.T) {
	e := &Engine{tracker: newTracker(), stopped: make(chan struct{})}
	defer close(e.stopped)
	u := newUDPTable(e)
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close() // every send now fails, as on a line that is gone
	remote := netip.MustParseAddrPort("198.51.100.7:4500")
	sess := &udpSession{key: "s", conn: conn, flows: map[netip.AddrPort]*udpFlow{}}
	f := &udpFlow{key: "10.0.0.2:4500>198.51.100.7:4500", sess: sess, remote: remote}
	u.mu.Lock()
	u.flows[f.key] = f
	u.mu.Unlock()
	sess.flows[remote] = f
	f.tr = e.tracker.add(&Metadata{Network: "udp", Process: "vpnclient.exe"}, []string{"LINK2"}, nil, udpForget{u, f})
	for i := 0; i < 2; i++ {
		u.write(f, []byte("hello"))
	}
	u.mu.Lock()
	_, kept := u.flows[f.key]
	u.mu.Unlock()
	if !kept {
		t.Fatal("two failures should not be enough to give up on the line")
	}
	u.write(f, []byte("hello"))
	u.mu.Lock()
	_, still := u.flows[f.key]
	u.mu.Unlock()
	if still || sess.flows[remote] != nil {
		t.Fatal("the flow is still remembered after its line stopped sending")
	}
	if n := len(e.tracker.snapshot()["connections"].([]map[string]any)); n != 0 {
		t.Fatalf("still listed: %d", n)
	}
}
