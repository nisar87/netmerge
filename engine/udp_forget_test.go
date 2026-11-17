package engine

import (
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
