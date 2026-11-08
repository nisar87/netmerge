package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type dlReq struct {
	Action      string   `json:"action"`
	ID          string   `json:"id"`
	URLs        []string `json:"urls"`
	DeleteFiles bool     `json:"deleteFiles"`
	dlAdd
	Config *dlConfig `json:"config"`
}

func registerDownloadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/downloads", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in dlReq
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in) != nil {
				writeErr(w, 400, "bad request")
				return
			}
			out, err := dlMgr.handle(in)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			if out != nil {
				writeJSON(w, out)
				return
			}
		}
		writeJSON(w, dlMgr.snapshot())
	}))
}

func (m *dlManager) snapshot() map[string]any {
	act, sp := m.summary()
	var lines []map[string]any
	for _, l := range m.lines() {
		lines = append(lines, map[string]any{"name": l.Name, "kind": l.Kind})
	}
	return map[string]any{"items": m.views(), "config": m.config(), "active": act, "speed": sp, "lines": lines, "knownSites": m.knownHosts()}
}

func (m *dlManager) handle(in dlReq) (map[string]any, error) {
	switch in.Action {
	case "probe":
		return m.probe(in.URL, in.Header)
	case "add":
		urls := in.URLs
		if in.URL != "" {
			urls = append([]string{in.URL}, urls...)
		}
		if len(urls) == 0 {
			return nil, errors.New("paste at least one link")
		}
		if len(urls) > 500 {
			return nil, errors.New("too many links at once (500 at most)")
		}
		var added []*dlItem
		for _, u := range urls {
			a := in.dlAdd
			a.URL = u
			if len(urls) > 1 {
				a.Name = ""
			}
			it, err := m.add(a)
			if err != nil {
				if len(urls) == 1 {
					return nil, err
				}
				continue
			}
			added = append(added, it)
		}
		if len(added) == 0 {
			return nil, errors.New("none of the links could be used")
		}
		return map[string]any{"added": added, "items": m.views(), "config": m.config()}, nil
	case "pause":
		return nil, m.pause(in.ID)
	case "resume":
		return nil, m.resume(in.ID)
	case "retry":
		return nil, m.resume(in.ID)
	case "remove":
		return nil, m.remove(in.ID, in.DeleteFiles)
	case "clear":
		m.clearFinished()
		return nil, nil
	case "pauseAll":
		m.forAll(func(id string) { _ = m.pause(id) })
		return nil, nil
	case "resumeAll":
		m.forAll(func(id string) { _ = m.resume(id) })
		return nil, nil
	case "open", "folder":
		return nil, m.openItem(in.ID, in.Action == "folder")
	case "forgetHosts":
		m.forgetHosts()
		return nil, nil
	case "config":
		if in.Config == nil {
			return nil, errors.New("missing settings")
		}
		return nil, m.setConfig(*in.Config)
	}
	return nil, errors.New("unknown action")
}

func (m *dlManager) forAll(f func(id string)) {
	m.mu.Lock()
	var ids []string
	for _, e := range m.items {
		if e.State != "done" {
			ids = append(ids, e.ID)
		}
	}
	m.mu.Unlock()
	for _, id := range ids {
		f(id)
	}
}

func (m *dlManager) openItem(id string, reveal bool) error {
	m.mu.Lock()
	e := m.find(id)
	var p, state string
	if e != nil {
		p, state = e.Path, e.State
	}
	m.mu.Unlock()
	if e == nil || p == "" {
		return errors.New("the file is not there")
	}
	if state != "done" {
		p = dlPartPath(p)
		reveal = true
	}
	if _, err := os.Stat(p); err != nil {
		if reveal {
			return openPath(filepath.Dir(p), false)
		}
		return errors.New("the file was moved or deleted")
	}
	return openPath(p, reveal)
}

// probe: what is behind this link? Used by the "add" dialog.
func (m *dlManager) probe(link string, hdr map[string]string) (map[string]any, error) {
	link, err := dlCleanURL(link)
	if err != nil {
		return nil, err
	}
	lines := m.lines()
	if len(lines) == 0 {
		return nil, errors.New("no internet connection is available right now")
	}
	h := dlCleanHeader(hdr)
	private := dlLocalHost(dlHostOf(link))
	var info dlInfo
	var perr error
	for _, l := range lines {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		info, perr = dlProbe(ctx, dlLineClient(l, private), link, h)
		cancel()
		if perr == nil {
			break
		}
	}
	if perr != nil {
		msg, _ := dlFriendly(perr)
		return map[string]any{"ok": false, "error": msg}, nil
	}
	cfg := m.config()
	dir := cfg.Dir
	if cfg.UseCategories {
		dir = filepath.Join(dir, dlCategory(info.Name))
	}
	return map[string]any{"ok": true, "info": info, "dir": dir, "category": dlCategory(info.Name), "lines": len(lines),
		"locked": m.HostLocked(dlHostOf(link))}, nil
}

func (m *dlManager) knownHosts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, v := range m.hosts {
		if v > time.Now().Unix() {
			n++
		}
	}
	return n
}
