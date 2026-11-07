package main

import (
	"net/http"
	"sync"
	"time"
)

// NetMerge checks each connection itself (a small request out of that adapter every few seconds) instead of
// leaving it to the engine. The engine's own health check used to mark a connection "dead" when it was busy
// downloading, and then sent every new download to the other link only.
type probeState struct {
	ok    bool
	delay int
	fails int
	dead  bool
}

var (
	probeMu sync.Mutex
	probes  = map[string]probeState{}
)

var probeURLs = []string{
	"http://www.gstatic.com/generate_204",
	"http://connectivitycheck.gstatic.com/generate_204",
	"http://www.msftconnecttest.com/connecttest.txt",
}

// isDead reports a connection that has had no working internet for a while (and no traffic) - it is left out
// of the engine's groups until it answers again.
func isDead(name string) bool {
	probeMu.Lock()
	defer probeMu.Unlock()
	return probes[name].dead
}

func healthLoop() {
	time.Sleep(8 * time.Second)
	t := time.NewTicker(10 * time.Second)
	for {
		safely("probe", probeAll)
		<-t.C
	}
}

func probeAll() {
	s := getSettings()
	present := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		present[a.Name] = a
	}
	var wg sync.WaitGroup
	for _, l := range s.Links {
		a, ok := present[l.Name]
		if !ok || !l.Enabled {
			probeMu.Lock()
			delete(probes, l.Name)
			probeMu.Unlock()
			continue
		}
		wg.Add(1)
		go func(name string, a Adapter) {
			defer wg.Done()
			safely("probe "+name, func() { probeOne(name, a) })
		}(l.Name, a)
	}
	wg.Wait()
}

func probeOne(name string, a Adapter) {
	c := clientFor(&a)
	c.Timeout = 7 * time.Second
	ok, delay := false, 0
	for _, u := range probeURLs {
		start := time.Now()
		req, _ := http.NewRequest("GET", u, nil)
		resp, err := c.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 204 || resp.StatusCode == 200 {
			ok, delay = true, int(time.Since(start).Milliseconds())
			break
		}
	}
	statsMu.Lock()
	rx := 0.0
	if tr := tracks[name]; tr != nil {
		rx = tr.rx
		if tr.tx > rx {
			rx = tr.tx
		}
	}
	statsMu.Unlock()

	probeMu.Lock()
	st := probes[name]
	was := st.dead
	if ok {
		st = probeState{ok: true, delay: delay}
	} else if rx >= 12500 {
		// a link that is carrying traffic (either direction) obviously works: a probe that fails while the line is
		// full only waited in the queue, so it does not count against the link
		st = probeState{ok: true, delay: st.delay}
	} else {
		st.ok = false
		st.fails++
		if st.fails >= 4 {
			st.dead = true
		}
	}
	probes[name] = st
	probeMu.Unlock()
	if st.dead != was {
		if st.dead {
			trayBalloon("Connection down", labelFor(getSettings(), name)+" is not reaching the internet. It is left out until it recovers.")
		} else {
			trayBalloon("Connection back", labelFor(getSettings(), name)+" is working again.")
		}
		logf("connection %s is %s", name, map[bool]string{true: "not reaching the internet - left out until it recovers", false: "back online"}[st.dead])
	}
	statsMu.Lock()
	health[name] = linkHealth{ok: st.ok, delay: st.delay, fails: st.fails}
	statsMu.Unlock()
}
