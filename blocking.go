package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed filters/starter.txt
var starterList string

// BlockSettings is the on/off state of ad & tracker blocking and which optional packs are switched on.
type BlockSettings struct {
	Enabled bool            `json:"enabled"`
	Packs   map[string]bool `json:"packs"`
}

func defaultBlock() *BlockSettings { return &BlockSettings{Enabled: true, Packs: map[string]bool{}} }

func blockCfg(s Settings) BlockSettings {
	if s.Block == nil {
		return *defaultBlock()
	}
	b := *s.Block
	if b.Packs == nil {
		b.Packs = map[string]bool{}
	}
	return b
}

type packDef struct {
	ID, Name, Desc string
	URLs           []string // tried in order until one works
}

// Optional lists that NetMerge downloads and refreshes by itself. They are plain public lists of domains.
var packDefs = []packDef{
	{"ads", "Ads & trackers", "A large, regularly maintained community list of ad and tracking domains (HaGeZi Light, about 90,000 sites).",
		[]string{"https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/wildcard/light-onlydomains.txt",
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/light-onlydomains.txt"}},
	{"pro", "Ads, trackers & more (stronger)", "A bigger list that also blocks more telemetry and annoyances (HaGeZi Pro, about 220,000 sites). May break a few sites - use \"Allow\" on the Blocked today list if one does.",
		[]string{"https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/wildcard/pro-onlydomains.txt",
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/pro-onlydomains.txt"}},
	{"malware", "Malware & phishing", "Domains used for malware, phishing and scams (HaGeZi Threat Intelligence Mini, about 195,000 sites).",
		[]string{"https://cdn.jsdelivr.net/gh/hagezi/dns-blocklists@latest/wildcard/tif.mini-onlydomains.txt",
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/tif.mini-onlydomains.txt"}},
}

// Never blocked, whatever the lists say: things NetMerge, Windows and the lists themselves depend on.
var neverBlock = []string{
	"www.gstatic.com", "connectivitycheck.gstatic.com", "msftconnecttest.com", "msftncsi.com",
	"raw.githubusercontent.com", "github.com", "cdn.jsdelivr.net",
}

func filtersDataDir() string   { return filepath.Join(rootDir, "data", "filters") }
func packsDir() string         { return filepath.Join(filtersDataDir(), "packs") }
func engineFiltersDir() string { return filepath.Join(configDir(), "filters") }

// ---- parsing ----

var domainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9_-]{0,61}[a-z0-9])?\.)+[a-z][a-z0-9-]{0,61}[a-z0-9]$`)

func validDomain(d string) bool {
	return len(d) <= 253 && domainRe.MatchString(d)
}

// parseDomains reads a block list in any common format (one domain per line, hosts file, or Adblock "||domain^").
func parseDomains(r io.Reader) []string {
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '!' || line[0] == '[' || line[0] == ';' {
			continue
		}
		if i := strings.IndexByte(line, '#'); i > 0 {
			line = strings.TrimSpace(line[:i])
		}
		var d string
		switch {
		case strings.HasPrefix(line, "||"):
			d = strings.TrimPrefix(line, "||")
			if i := strings.IndexAny(d, "^/$*"); i >= 0 {
				if d[i] != '^' || strings.ContainsAny(d[i:], "/*") {
					continue
				}
				d = d[:i]
			}
		default:
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			if net.ParseIP(f[0]) != nil {
				if len(f) < 2 {
					continue
				}
				d = f[1]
			} else {
				d = f[0]
			}
		}
		d = strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(strings.Trim(d, "."), "*."), "+."))
		if d == "localhost" || !validDomain(d) || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}

// ---- user lists ----

func userListPath(name string) string { return filepath.Join(filtersDataDir(), name) }

func readUserList(name string) []string {
	f, err := os.Open(userListPath(name))
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseDomains(f)
}

func writeUserList(name string, domains []string) error {
	_ = os.MkdirAll(filtersDataDir(), 0o755)
	body := "# One domain per line. Subdomains are included.\n" + strings.Join(domains, "\n") + "\n"
	return atomicWrite(userListPath(name), []byte(body))
}

func atomicWrite(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---- packs ----

type packMeta struct {
	Updated int64  `json:"updated"`
	Count   int    `json:"count"`
	Error   string `json:"error"`
}

var (
	packMu   sync.Mutex
	updating = map[string]bool{}
)

func packFile(id string) string { return filepath.Join(packsDir(), id+".txt") }
func metaFile(id string) string { return filepath.Join(packsDir(), id+".json") }

func readMeta(id string) packMeta {
	var m packMeta
	if b, err := os.ReadFile(metaFile(id)); err == nil {
		logErr("read block list details "+id, json.Unmarshal(b, &m))
	}
	return m
}

func writeMeta(id string, m packMeta) {
	b, _ := json.Marshal(m)
	_ = os.MkdirAll(packsDir(), 0o755)
	_ = atomicWrite(metaFile(id), b)
}

func findPack(id string) *packDef {
	for i := range packDefs {
		if packDefs[i].ID == id {
			return &packDefs[i]
		}
	}
	return nil
}

var dlClient = &http.Client{Timeout: 120 * time.Second}

// updatePack downloads one pack, checks it looks right, and only then replaces the saved copy,
// so a failed or damaged download never weakens or breaks the filters you already have.
func updatePack(id string) error {
	p := findPack(id)
	if p == nil {
		return fmt.Errorf("unknown list")
	}
	packMu.Lock()
	if updating[id] {
		packMu.Unlock()
		return fmt.Errorf("already updating")
	}
	updating[id] = true
	packMu.Unlock()
	defer func() { packMu.Lock(); delete(updating, id); packMu.Unlock() }()

	fail := func(err error) error {
		m := readMeta(id)
		m.Error = err.Error()
		writeMeta(id, m)
		logf("filter list %q update failed: %v", id, err)
		return err
	}
	var domains []string
	var lastErr error
	for _, u := range p.URLs {
		domains, lastErr = fetchDomains(u)
		if lastErr == nil {
			break
		}
		logf("filter list %q: %s: %v", id, u, lastErr)
	}
	if lastErr != nil {
		return fail(lastErr)
	}
	_ = os.MkdirAll(packsDir(), 0o755)
	if err := atomicWrite(packFile(id), []byte(strings.Join(domains, "\n")+"\n")); err != nil {
		return fail(err)
	}
	writeMeta(id, packMeta{Updated: time.Now().Unix(), Count: len(domains)})
	logf("filter list %q updated: %d domains", id, len(domains))
	return nil
}

func fetchDomains(url string) ([]string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "NetMerge")
	resp, err := dlClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not download (%v)", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed (HTTP %d)", resp.StatusCode)
	}
	domains := parseDomains(io.LimitReader(resp.Body, 60<<20))
	if len(domains) < 50 {
		return nil, fmt.Errorf("the downloaded list looked wrong (%d entries) - kept the old copy", len(domains))
	}
	return domains, nil
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

func readPack(id string) []string {
	f, err := os.Open(packFile(id))
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseDomains(f)
}

// ---- building the engine's files ----

func writeProviderFile(name string, domains []string) error {
	_ = os.MkdirAll(engineFiltersDir(), 0o755)
	if len(domains) == 0 {
		domains = []string{"nm-placeholder.invalid"} // an empty rule set is not accepted by the engine
	}
	var sb strings.Builder
	sb.Grow(len(domains) * 24)
	for _, d := range domains {
		sb.WriteString("+.")
		sb.WriteString(d)
		sb.WriteByte('\n')
	}
	return atomicWrite(filepath.Join(engineFiltersDir(), name), []byte(sb.String()))
}

var buildMu sync.Mutex

// rebuildFilters merges the starter list, enabled packs and the user's block list into the files the engine reads.
func rebuildFilters(s Settings) error {
	buildMu.Lock()
	defer buildMu.Unlock()
	bs := blockCfg(s)
	set := map[string]struct{}{}
	add := func(list []string) {
		for _, d := range list {
			set[d] = struct{}{}
		}
	}
	add(parseDomains(strings.NewReader(starterList)))
	for _, p := range packDefs {
		if bs.Packs[p.ID] {
			add(readPack(p.ID))
		}
	}
	add(readUserList("blocklist.txt"))
	all := make([]string, 0, len(set))
	for d := range set {
		all = append(all, d)
	}
	sort.Strings(all)
	if err := writeProviderFile("nm_block.txt", all); err != nil {
		return err
	}
	allow := append(append([]string{}, neverBlock...), readUserList("allowlist.txt")...)
	return writeProviderFile("nm_allow.txt", allow)
}

// ---- live statistics (read from the engine's log) ----

// BlockEvent is one blocked website, with how often it was blocked today and when last.
type BlockEvent struct {
	Time  int64  `json:"time"`
	Host  string `json:"host"`
	App   string `json:"app"`
	Count int    `json:"count"`
}

type blockStatsT struct {
	mu     sync.Mutex
	Day    string `json:"day"`
	Today  int    `json:"today"`
	Total  int    `json:"total"`
	recent map[string]*BlockEvent
	top    map[string]int
	dirty  bool
}

var bstats = &blockStatsT{top: map[string]int{}, recent: map[string]*BlockEvent{}}

// The engine logs a refused connection as "match AND(((RuleSet,nm_block) && (NOT,(!(RuleSet,nm_allow))))) using
// REJECT"; the plain "match RuleSet(nm_block)" form still counts.
// The program name in brackets after the source is there only for programs on this PC: a shared phone's line is
// "[TCP] 192.168.1.107:60372 --> 172.217.117.4:443 match ..." (seen on a real PC), so the brackets are optional.
var blockedRe = regexp.MustCompile(`\[(?:TCP|UDP)\] ([^\s(]+)(?:\(([^)]*)\))? --> (\S+) match (?:RuleSet\(nm_block\)|AND\(\(\(RuleSet,nm_block\))`)

// blockedApp names who asked: the program on this PC, or the shared device by its address.
func blockedApp(src, app string) string {
	if app != "" {
		return app
	}
	ip, _, err := net.SplitHostPort(src)
	if err != nil {
		ip = src
	}
	if ip == "" || strings.HasPrefix(ip, "198.18.") || ip == "127.0.0.1" || ip == "::1" {
		return "" // a program on this PC whose name the engine could not find
	}
	return "Shared device (" + ip + ")"
}

func (b *blockStatsT) rollLocked() {
	d := time.Now().Format("2006-01-02")
	if b.Day != d {
		b.Day, b.Today = d, 0
		b.top = map[string]int{}
		b.recent = map[string]*BlockEvent{}
		b.dirty = true
	}
}

func (b *blockStatsT) add(app, hostport string) {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	b.Today++
	b.Total++
	b.dirty = true
	if len(b.top) > 3000 {
		b.top = map[string]int{}
	}
	b.top[host]++
	if len(b.recent) > 1500 {
		b.recent = map[string]*BlockEvent{}
	}
	if e := b.recent[host]; e != nil {
		e.Count++
		e.Time, e.App = time.Now().Unix(), app
	} else {
		b.recent[host] = &BlockEvent{Time: time.Now().Unix(), Host: host, App: app, Count: 1}
	}
}

func (b *blockStatsT) snapshot() (today, total int, recent []BlockEvent, top []map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked()
	today, total = b.Today, b.Total
	for _, e := range b.recent {
		recent = append(recent, *e)
	}
	// most-blocked first: the order only changes when the counts really change, so the list stays calm
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].Count != recent[j].Count {
			return recent[i].Count > recent[j].Count
		}
		return recent[i].Host < recent[j].Host
	})
	if len(recent) > 20 {
		recent = recent[:20]
	}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range b.top {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v })
	for i, x := range kvs {
		if i >= 10 {
			break
		}
		top = append(top, map[string]any{"host": x.k, "count": x.v})
	}
	return
}

func statsPath() string { return filepath.Join(rootDir, "data", "blockstats.json") }

func (b *blockStatsT) load() {
	if raw, err := os.ReadFile(statsPath()); err == nil {
		b.mu.Lock()
		logErr("read the blocking statistics", json.Unmarshal(raw, b))
		b.rollLocked()
		b.mu.Unlock()
	}
}

func (b *blockStatsT) save() {
	b.mu.Lock()
	if !b.dirty {
		b.mu.Unlock()
		return
	}
	raw, _ := json.Marshal(b)
	b.dirty = false
	b.mu.Unlock()
	_ = atomicWrite(statsPath(), raw)
}

// pumpEngineLog reads the engine's output. Normal "connection opened" lines are very frequent, so they are not
// written to disk (unless detailed logging is on); the ones that were blocked are counted for the Blocking page.
func pumpEngineLog(r *os.File, lf *os.File) {
	defer r.Close()
	if lf != nil {
		defer lf.Close()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	// the engine waits while we read its output, so this loop must stay very cheap: settings are re-read only every 2 s
	debug, checked := getSettings().DebugLog, time.Now()
	written := int64(0)
	if lf != nil {
		if st, err := lf.Stat(); err == nil {
			written = st.Size()
		}
	}
	write := func(line string) {
		if lf == nil {
			return
		}
		// keep the log from growing without limit while the engine runs for days (debug mode writes every connection)
		if written > 16<<20 {
			_ = lf.Truncate(0)
			written = 0
		}
		n, _ := lf.WriteString(line + "\n")
		written += int64(n)
	}
	for sc.Scan() {
		line := sc.Text()
		if time.Since(checked) > 2*time.Second {
			debug, checked = getSettings().DebugLog, time.Now()
		}
		noteEngineLine(line)
		if !strings.Contains(line, "REJECT") && !strings.Contains(line, `msg="[TCP] `) && !strings.Contains(line, `msg="[UDP] `) {
			write(line)
			continue
		}
		if m := blockedRe.FindStringSubmatch(line); m != nil {
			bstats.add(blockedApp(m[1], m[2]), m[3])
			if !debug {
				continue
			}
		} else if !debug && (strings.Contains(line, `msg="[TCP] `) || strings.Contains(line, `msg="[UDP] `)) {
			continue
		}
		write(line)
	}
}

// ---- background updater ----

func blockUpdater() {
	bstats.load()
	t := time.NewTicker(10 * time.Minute)
	saveT := time.NewTicker(time.Minute)
	first := time.After(20 * time.Second)
	for {
		select {
		case <-saveT.C:
			bstats.save()
		case <-first:
			safely("filter update", func() { updateDuePacks(false) })
		case <-t.C:
			safely("filter update", func() { updateDuePacks(false) })
		}
	}
}

// updateDuePacks refreshes enabled packs that are missing or older than a day (force = all enabled packs).
func updateDuePacks(force bool) {
	s := getSettings()
	bs := blockCfg(s)
	if !bs.Enabled {
		return
	}
	changed := false
	for _, p := range packDefs {
		if !bs.Packs[p.ID] {
			continue
		}
		m := readMeta(p.ID)
		if !force && m.Updated > 0 && time.Since(time.Unix(m.Updated, 0)) < 24*time.Hour {
			continue
		}
		if !force && m.Updated == 0 && m.Error != "" && time.Since(time.Unix(lastTry(p.ID), 0)) < time.Hour {
			continue
		}
		setLastTry(p.ID)
		if err := updatePack(p.ID); err == nil {
			changed = true
		}
	}
	if changed {
		if err := engine.Apply(); err != nil {
			logf("could not apply updated lists: %v", err)
		}
	}
}

var (
	tryMu sync.Mutex
	tries = map[string]int64{}
)

func lastTry(id string) int64 { tryMu.Lock(); defer tryMu.Unlock(); return tries[id] }
func setLastTry(id string)    { tryMu.Lock(); tries[id] = time.Now().Unix(); tryMu.Unlock() }

// ---- API ----

func blockSummary() map[string]any {
	bs := blockCfg(getSettings())
	today, total, _, _ := bstats.snapshot()
	return map[string]any{"enabled": bs.Enabled, "today": today, "total": total}
}

func blockState() map[string]any {
	s := getSettings()
	bs := blockCfg(s)
	today, total, recent, top := bstats.snapshot()
	packMu.Lock()
	upd := map[string]bool{}
	for k, v := range updating {
		upd[k] = v
	}
	packMu.Unlock()
	var packs []map[string]any
	n := len(parseDomains(strings.NewReader(starterList)))
	packs = append(packs, map[string]any{"id": "starter", "name": "Starter list", "desc": "Built into NetMerge: the biggest ad networks and trackers. Always on while blocking is on.",
		"enabled": true, "builtin": true, "count": n})
	for _, p := range packDefs {
		m := readMeta(p.ID)
		packs = append(packs, map[string]any{"id": p.ID, "name": p.Name, "desc": p.Desc, "enabled": bs.Packs[p.ID],
			"count": m.Count, "updated": m.Updated, "error": m.Error, "updating": upd[p.ID]})
	}
	join := func(l []string) string { return strings.Join(l, "\n") }
	return map[string]any{
		"enabled": bs.Enabled, "packs": packs,
		"blocklist": join(readUserList("blocklist.txt")), "allowlist": join(readUserList("allowlist.txt")),
		"today": today, "total": total, "recent": recent, "top": top,
	}
}

type blockReq struct {
	Enabled   *bool           `json:"enabled"`
	Packs     map[string]bool `json:"packs"`
	BlockList *string         `json:"blocklist"`
	AllowList *string         `json:"allowlist"`
}

func applyBlockReq(in blockReq) error {
	if in.BlockList != nil {
		if err := writeUserList("blocklist.txt", parseDomains(strings.NewReader(*in.BlockList))); err != nil {
			return err
		}
	}
	if in.AllowList != nil {
		if err := writeUserList("allowlist.txt", parseDomains(strings.NewReader(*in.AllowList))); err != nil {
			return err
		}
	}
	var needDownload []string
	updateSettings(func(s *Settings) {
		if s.Block == nil {
			s.Block = defaultBlock()
		}
		if s.Block.Packs == nil {
			s.Block.Packs = map[string]bool{}
		}
		if in.Enabled != nil {
			s.Block.Enabled = *in.Enabled
		}
		for id, on := range in.Packs {
			if findPack(id) == nil {
				continue
			}
			if on && !s.Block.Packs[id] && readMeta(id).Count == 0 {
				needDownload = append(needDownload, id)
			}
			s.Block.Packs[id] = on
		}
	})
	if err := engine.Apply(); err != nil {
		return err
	}
	if len(needDownload) > 0 {
		go safely("pack download", func() { updateDuePacks(false) })
	}
	return nil
}

func allowDomain(domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if !validDomain(domain) {
		return fmt.Errorf("that does not look like a domain name")
	}
	cur := readUserList("allowlist.txt")
	for _, d := range cur {
		if d == domain {
			return nil
		}
	}
	if err := writeUserList("allowlist.txt", append(cur, domain)); err != nil {
		return err
	}
	return engine.Apply()
}

func registerBlockRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/block", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var in blockReq
			if err := json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(&in); err != nil {
				writeErr(w, 400, "bad request")
				return
			}
			if err := applyBlockReq(in); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		writeJSON(w, blockState())
	}))
	mux.HandleFunc("/api/block/allow", action(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Domain string `json:"domain"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if err := allowDomain(in.Domain); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, blockState())
	}))
	mux.HandleFunc("/api/block/update", action(func(w http.ResponseWriter, r *http.Request) {
		go safely("pack update", func() { updateDuePacks(true) })
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("/api/fixconflict", action(func(w http.ResponseWriter, r *http.Request) {
		if err := fixConflicts(); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))
}
