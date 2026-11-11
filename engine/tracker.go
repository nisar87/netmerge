package engine

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Tracker keeps the list of open connections with their byte counts (the API's /connections).
type Tracker struct {
	mu        sync.RWMutex
	conns     map[string]*tracked
	upTotal   atomic.Int64
	downTotal atomic.Int64
}

type tracked struct {
	id       string
	meta     Metadata
	chains   []string
	rule     string
	payload  string
	start    time.Time
	up, down atomic.Int64
	closer   io.Closer
}

func newTracker() *Tracker { return &Tracker{conns: map[string]*tracked{}} }

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (t *Tracker) add(m *Metadata, chains []string, r *Rule, c io.Closer) *tracked {
	tr := &tracked{id: newID(), meta: *m, chains: chains, start: time.Now(), closer: c}
	if r != nil {
		tr.rule, tr.payload = r.Name(), r.Payload()
	}
	t.mu.Lock()
	t.conns[tr.id] = tr
	t.mu.Unlock()
	return tr
}

func (t *Tracker) remove(tr *tracked) {
	t.mu.Lock()
	delete(t.conns, tr.id)
	t.mu.Unlock()
}

func (t *Tracker) addUp(tr *tracked, n int64) {
	tr.up.Add(n)
	t.upTotal.Add(n)
}

func (t *Tracker) addDown(tr *tracked, n int64) {
	tr.down.Add(n)
	t.downTotal.Add(n)
}

func (t *Tracker) closeAll() {
	t.mu.RLock()
	var cs []io.Closer
	for _, c := range t.conns {
		if c.closer != nil {
			cs = append(cs, c.closer)
		}
	}
	t.mu.RUnlock()
	for _, c := range cs {
		_ = c.Close()
	}
}

func (t *Tracker) closeID(id string) bool {
	t.mu.RLock()
	c, ok := t.conns[id]
	t.mu.RUnlock()
	if ok && c.closer != nil {
		_ = c.closer.Close()
	}
	return ok
}

// snapshot is the JSON NetMerge reads (field names as mihomo writes them).
func (t *Tracker) snapshot() map[string]any {
	t.mu.RLock()
	list := make([]map[string]any, 0, len(t.conns))
	for _, c := range t.conns {
		m := c.meta
		host := m.Host
		if host == "" {
			host = m.SniffHost
		}
		dst := ""
		if m.DstIP.IsValid() {
			dst = m.DstIP.Unmap().String()
		}
		src := ""
		if m.SrcIP.IsValid() {
			src = m.SrcIP.Unmap().String()
		}
		list = append(list, map[string]any{
			"id": c.id,
			"metadata": map[string]any{
				"network": m.Network, "type": m.Type,
				"sourceIP": src, "sourcePort": strconv.Itoa(int(m.SrcPort)),
				"destinationIP": dst, "destinationPort": strconv.Itoa(int(m.DstPort)),
				"host": host, "sniffHost": m.SniffHost, "process": m.Process, "processPath": m.ProcessPath,
			},
			"upload": c.up.Load(), "download": c.down.Load(), "start": c.start.Format(time.RFC3339Nano),
			"chains": c.chains, "rule": c.rule, "rulePayload": c.payload,
		})
	}
	t.mu.RUnlock()
	return map[string]any{"downloadTotal": t.downTotal.Load(), "uploadTotal": t.upTotal.Load(), "connections": list}
}

// ---- relaying ----

var bufPool = sync.Pool{New: func() any { b := make([]byte, 32*1024); return &b }}

type closeWriter interface{ CloseWrite() error }

// relay copies both ways until both directions end, counting bytes. a is the side towards the app, b the website.
func (e *Engine) relay(a, b net.Conn, tr *tracked) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn, count func(int64)) {
		bp := bufPool.Get().(*[]byte)
		buf := *bp
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
				count(int64(n))
			}
			if err != nil {
				break
			}
		}
		bufPool.Put(bp)
		if cw, ok := dst.(closeWriter); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.SetReadDeadline(time.Now().Add(5 * time.Second))
		}
		done <- struct{}{}
	}
	go cp(b, a, func(n int64) { e.tracker.addUp(tr, n) })
	go cp(a, b, func(n int64) { e.tracker.addDown(tr, n) })
	<-done
	// one side finished: give the other a moment to finish too, then close both
	t := time.NewTimer(30 * time.Second)
	select {
	case <-done:
	case <-t.C:
	}
	t.Stop()
	_ = a.Close()
	_ = b.Close()
}
