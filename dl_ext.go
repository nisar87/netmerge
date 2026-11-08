package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The browser extension talks to NetMerge on the same local port as the window, under /ext/. Only extension pages may
// call it (a normal website is refused by its Origin), the Host header must be local, and every call except "ping"
// and "pair" needs a private token that the extension got by typing a one-time code shown in NetMerge.

type extBrowser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hash     string `json:"hash"`
	Created  int64  `json:"created"`
	LastSeen int64  `json:"lastSeen"`
}

type extStore struct {
	mu       sync.Mutex
	path     string
	Browsers []extBrowser `json:"browsers"`
	MinMB    int          `json:"minMB"`
	Off      bool         `json:"off"`
	code     string
	codeExp  time.Time
	tries    int
	failures []time.Time
}

var ext = &extStore{MinMB: 5}

func extFile() string { return filepath.Join(rootDir, "data", "extension.json") }

func (s *extStore) load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = extFile()
	if b, err := os.ReadFile(s.path); err == nil {
		_ = json.Unmarshal(b, s)
	}
	if s.MinMB < 0 || s.MinMB > 100000 {
		s.MinMB = 5
	}
}

func (s *extStore) saveLocked() {
	if s.path == "" {
		s.path = extFile()
	}
	b, _ := json.MarshalIndent(s, "", " ")
	_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
	_ = dlWriteFileAtomicMode(s.path, b, 0o600)
}

func extHash(tok string) string { h := sha256.Sum256([]byte(tok)); return hex.EncodeToString(h[:]) }

// newCode makes a one-time code that is valid for two minutes (a new one replaces the old one).
func (s *extStore) newCode() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := randomHex(5) // 10 hex letters, read out as groups of 5
	s.code, s.codeExp, s.tries = d, time.Now().Add(2*time.Minute), 0
	return d, s.codeExp
}

var errExtCode = errors.New("that code is wrong or has expired")

func (s *extStore) pair(code, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	code = strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(code))
	if s.code == "" || time.Now().After(s.codeExp) {
		s.code = ""
		return "", errExtCode
	}
	if subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		s.tries++
		if s.tries >= 5 {
			s.code = "" // too many guesses: the code is dead
		}
		return "", errExtCode
	}
	s.code = ""
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 40 {
		name = "Browser"
	}
	if len(s.Browsers) >= 8 {
		return "", errors.New("too many browsers are paired; remove one first")
	}
	tok := randomHex(32)
	s.Browsers = append(s.Browsers, extBrowser{ID: randomHex(4), Name: name, Hash: extHash(tok), Created: time.Now().Unix()})
	s.saveLocked()
	return tok, nil
}

func (s *extStore) check(tok string) bool {
	if tok == "" {
		return false
	}
	h := extHash(tok)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// too many wrong tokens in a minute: slow down guessing
	keep := s.failures[:0]
	for _, t := range s.failures {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	s.failures = keep
	if len(s.failures) >= 20 {
		return false
	}
	for i := range s.Browsers {
		if subtle.ConstantTimeCompare([]byte(h), []byte(s.Browsers[i].Hash)) == 1 {
			s.Browsers[i].LastSeen = now.Unix()
			return true
		}
	}
	s.failures = append(s.failures, now)
	return false
}

func (s *extStore) revoke(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []extBrowser
	for _, b := range s.Browsers {
		if b.ID != id {
			keep = append(keep, b)
		}
	}
	s.Browsers = keep
	s.saveLocked()
}

func (s *extStore) setConfig(minMB int, off bool) error {
	if minMB < 0 || minMB > 100000 {
		return errors.New("the minimum size must be between 0 and 100000 MB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.MinMB, s.Off = minMB, off
	s.saveLocked()
	return nil
}

func (s *extStore) view() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bs []map[string]any
	for _, b := range s.Browsers {
		bs = append(bs, map[string]any{"id": b.ID, "name": b.Name, "created": b.Created, "lastSeen": b.LastSeen})
	}
	return map[string]any{"browsers": bs, "minMB": s.MinMB, "enabled": !s.Off}
}

func extOrigin(o string) bool {
	return strings.HasPrefix(o, "chrome-extension://") || strings.HasPrefix(o, "moz-extension://") || strings.HasPrefix(o, "safari-web-extension://")
}

func extHandler(auth bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o := r.Header.Get("Origin")
		if o != "" && !extOrigin(o) {
			writeErr(w, 403, "forbidden")
			return
		}
		if o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		if auth {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ext.check(tok) {
				writeErr(w, 401, "not paired")
				return
			}
		}
		h(w, r)
	}
}

func registerExtRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ext/ping", extHandler(false, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"app": "netmerge", "version": appVersion})
	}))
	mux.HandleFunc("/ext/pair", extHandler(false, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "POST only")
			return
		}
		var in struct{ Code, Name string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
			writeErr(w, 400, "bad request")
			return
		}
		tok, err := ext.pair(in.Code, in.Name)
		if err != nil {
			writeErr(w, 403, err.Error())
			return
		}
		writeJSON(w, map[string]any{"token": tok})
	}))
	mux.HandleFunc("/ext/status", extHandler(true, func(w http.ResponseWriter, r *http.Request) {
		act, sp := dlMgr.summary()
		v := ext.view()
		writeJSON(w, map[string]any{"version": appVersion, "enabled": v["enabled"], "minMB": v["minMB"], "active": act, "speed": sp, "lines": len(dlMgr.lines())})
	}))
	mux.HandleFunc("/ext/unpair", extHandler(true, func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		h := extHash(tok)
		ext.mu.Lock()
		var id string
		for _, b := range ext.Browsers {
			if b.Hash == h {
				id = b.ID
			}
		}
		ext.mu.Unlock()
		if id != "" {
			ext.revoke(id)
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/ext/add", extHandler(true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "POST only")
			return
		}
		var in struct {
			URL       string `json:"url"`
			Referrer  string `json:"referrer"`
			Cookie    string `json:"cookie"`
			UserAgent string `json:"userAgent"`
			Filename  string `json:"filename"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
			writeErr(w, 400, "bad request")
			return
		}
		res, err := dlMgr.addFromBrowser(in.URL, in.Referrer, in.Cookie, in.UserAgent, in.Filename)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, res)
	}))
}

// addFromBrowser adds a link the browser was about to download. Local-network targets are refused.
func (m *dlManager) addFromBrowser(link, referrer, cookie, ua, name string) (map[string]any, error) {
	h := map[string]string{}
	if referrer != "" {
		h["Referer"] = referrer
	}
	if cookie != "" {
		h["Cookie"] = cookie
	}
	if ua != "" {
		h["User-Agent"] = ua
	}
	it, err := m.add(dlAdd{URL: link, Header: h, Name: name, FromWeb: true})
	if err != nil {
		if errors.Is(err, errDlDuplicate) {
			return map[string]any{"ok": true, "duplicate": true}, nil
		}
		return nil, err
	}
	return map[string]any{"ok": true, "id": it.ID}, nil
}
