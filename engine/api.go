package engine

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

type http_server = http.Server

// startAPI serves the local control API NetMerge uses (the same addresses and answers as mihomo's).
func (e *Engine) startAPI(addr, secret string) error {
	if e.api != nil && e.api.Addr == addr {
		return nil
	}
	if e.api != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = e.api.Shutdown(ctx)
		cancel()
		e.api = nil
	}
	if addr == "" {
		return nil
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		// "started" only once traffic really flows through the engine (the virtual adapter and its routes are
		// in place): NetMerge shows merging as on when this answers
		if !e.ready.Load() {
			writeJSON(w, 503, map[string]any{"message": "starting"})
			return
		}
		writeJSON(w, 200, map[string]any{"version": "nmengine-" + Version, "meta": true})
	})
	// POST /quit stops the engine cleanly (NetMerge uses it instead of killing the process, so the PC's
	// routes and DNS are always put back). mihomo has no such call; NetMerge only sends it to its own engine.
	mux.HandleFunc("POST /quit", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
		go e.Stop()
	})
	mux.HandleFunc("GET /proxies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"proxies": e.proxiesJSON()})
	})
	mux.HandleFunc("GET /proxies/{name}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := e.proxiesJSON()[r.PathValue("name")]
		if !ok {
			writeJSON(w, 404, map[string]string{"message": "resource not found"})
			return
		}
		writeJSON(w, 200, p)
	})
	mux.HandleFunc("PUT /proxies/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			writeJSON(w, 400, map[string]string{"message": "body invalid"})
			return
		}
		e.mu.RLock()
		o, ok := e.router.outs[r.PathValue("name")]
		e.mu.RUnlock()
		if !ok {
			writeJSON(w, 404, map[string]string{"message": "resource not found"})
			return
		}
		s, ok := o.(*Selector)
		if !ok {
			writeJSON(w, 400, map[string]string{"message": "must be a Selector"})
			return
		}
		if !s.Set(body.Name) {
			writeJSON(w, 400, map[string]string{"message": "proxy not exist"})
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /connections", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, e.tracker.snapshot())
	})
	mux.HandleFunc("DELETE /connections", func(w http.ResponseWriter, r *http.Request) {
		e.tracker.closeAll()
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /connections/{id}", func(w http.ResponseWriter, r *http.Request) {
		e.tracker.closeID(r.PathValue("id"))
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /configs", func(w http.ResponseWriter, r *http.Request) {
		c := e.config()
		writeJSON(w, 200, map[string]any{"mode": c.Mode, "log-level": c.LogLevel, "mixed-port": c.MixedPort, "allow-lan": c.AllowLAN, "ipv6": c.IPv6, "tun": map[string]any{"enable": c.Tun.Enable}})
	})
	mux.HandleFunc("PUT /configs", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Path string `json:"path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		path := body.Path
		if path == "" {
			path = e.dir + "/config.yaml"
		}
		if err := e.Load(path); err != nil {
			e.log.Errorf("reload failed: %v", err)
			writeJSON(w, 400, map[string]string{"message": err.Error()})
			return
		}
		e.log.Infof("settings reloaded")
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /traffic", func(w http.ResponseWriter, r *http.Request) {
		// one line per second: {"up":bytes,"down":bytes} since the last line
		fl, _ := w.(http.Flusher)
		w.Header().Set("Content-Type", "application/json")
		up, down := e.tracker.upTotal.Load(), e.tracker.downTotal.Load()
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-t.C:
				u, d := e.tracker.upTotal.Load(), e.tracker.downTotal.Load()
				b, _ := json.Marshal(map[string]int64{"up": u - up, "down": d - down})
				up, down = u, d
				if _, err := w.Write(append(b, '\n')); err != nil {
					return
				}
				if fl != nil {
					fl.Flush()
				}
			}
		}
	})

	srv := &http.Server{Addr: addr, Handler: e.authMiddleware(secret, mux), ReadHeaderTimeout: 5 * time.Second}
	e.api = srv
	go func() { _ = srv.Serve(l) }()
	return nil
}

func (e *Engine) authMiddleware(secret string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if secret != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
				writeJSON(w, 401, map[string]string{"message": "Unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// proxiesJSON is /proxies: every line and group with its type, members and current choice.
func (e *Engine) proxiesJSON() map[string]any {
	e.mu.RLock()
	r := e.router
	e.mu.RUnlock()
	out := map[string]any{}
	for _, n := range r.order {
		o := r.outs[n]
		p := map[string]any{"name": n, "type": o.Type(), "history": []any{}, "udp": true}
		switch x := o.(type) {
		case *Selector:
			p["now"] = x.Now()
			p["all"] = x.members
		case *LoadBalance:
			p["all"] = x.members
		case *Line:
			p["udp"] = x.udp
			p["alive"] = x.healthy()
		}
		out[n] = p
	}
	return out
}
