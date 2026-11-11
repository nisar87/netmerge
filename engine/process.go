package engine

import (
	"net/netip"
	"path/filepath"
	"sync"
	"time"
)

// processFinder answers "which program owns this local address and port". The operating system's connection
// table is read at most a few times a second; answers are kept for a little while.
type processFinder struct {
	mu       sync.Mutex
	cache    map[string]procHit
	pidName  map[uint32]procName
	lastRead time.Time
}

type procHit struct {
	name, path string
	at         time.Time
}

type procName struct {
	path string
	at   time.Time
}

func newProcessFinder() *processFinder {
	return &processFinder{cache: map[string]procHit{}, pidName: map[uint32]procName{}}
}

func (p *processFinder) find(network string, ip netip.Addr, port uint16) (string, string) {
	key := network + "|" + netip.AddrPortFrom(ip, port).String()
	p.mu.Lock()
	if h, ok := p.cache[key]; ok && time.Since(h.at) < 3*time.Second {
		p.mu.Unlock()
		return h.name, h.path
	}
	p.mu.Unlock()
	pid, ok := lookupOwner(network, ip, port)
	if !ok {
		return "", ""
	}
	path := p.pathOf(pid)
	name := filepath.Base(path)
	if path == "" {
		name = ""
	}
	p.mu.Lock()
	if len(p.cache) > 4096 {
		p.cache = map[string]procHit{}
	}
	p.cache[key] = procHit{name: name, path: path, at: time.Now()}
	p.mu.Unlock()
	return name, path
}

func (p *processFinder) pathOf(pid uint32) string {
	p.mu.Lock()
	if n, ok := p.pidName[pid]; ok && time.Since(n.at) < 30*time.Second {
		p.mu.Unlock()
		return n.path
	}
	p.mu.Unlock()
	path := processPath(pid)
	p.mu.Lock()
	if len(p.pidName) > 2048 {
		p.pidName = map[uint32]procName{}
	}
	p.pidName[pid] = procName{path: path, at: time.Now()}
	p.mu.Unlock()
	return path
}
