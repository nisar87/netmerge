package main

// The download manager: the list of downloads, the queue, what is kept on disk, and the connection to the lines.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type dlConfig struct {
	Dir           string `json:"dir"`
	UseCategories bool   `json:"useCategories"`
	MaxActive     int    `json:"maxActive"`
	MaxPerLine    int    `json:"maxPerLine"`
	AllowMobile   bool   `json:"allowMobile"`
}

type dlItem struct {
	ID       string            `json:"id"`
	URL      string            `json:"url"`
	Header   map[string]string `json:"header,omitempty"`
	Name     string            `json:"name"`
	Dir      string            `json:"dir"` // chosen by the person ("" = default)
	Path     string            `json:"path"`
	Category string            `json:"category"`
	Size     int64             `json:"size"`
	Done     int64             `json:"done"`
	State    string            `json:"state"` // queued | downloading | paused | done | error
	Err      string            `json:"err"`
	Note     string            `json:"note"`
	Lines    []string          `json:"lines"`
	Added    int64             `json:"added"`
	Finished int64             `json:"finished"`
	Retries  int               `json:"retries"`
	RetryAt  int64             `json:"retryAt"`
	Bytes    map[string]int64  `json:"bytes"` // how much each line carried
}

type dlEntry struct {
	dlItem
	cancel    context.CancelFunc
	task      *dlTask
	wantPause bool
	running   bool
	prog      dlProgress
	doneCh    chan struct{}
}

type dlView struct {
	dlItem
	Header   struct{}     `json:"-"`
	Speed    float64      `json:"speed"`
	LineStat []dlLineStat `json:"lineStats"`
	Order    int          `json:"order"`
}

type dlManager struct {
	mu     sync.Mutex
	items  []*dlEntry
	cfg    dlConfig
	hosts  map[string]int64
	dirty  bool
	global dlBucket
	perLn  map[string]*dlBucket
	lines  func() []dlLine
	seq    int
	quit   bool
	wg     sync.WaitGroup
	loopWG sync.WaitGroup
}

var dlMgr *dlManager

func init() { dlMgr = newDlManager(dlProdLines) }

func newDlManager(lines func() []dlLine) *dlManager {
	m := &dlManager{hosts: map[string]int64{}, perLn: map[string]*dlBucket{}, lines: lines}
	m.cfg = dlConfig{Dir: defaultDlDir(), UseCategories: true, MaxActive: 3, MaxPerLine: 8}
	return m
}

func defaultDlDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, "Downloads")
	}
	return filepath.Join(rootDir, "downloads")
}

func dlFile() string { return filepath.Join(rootDir, "data", "downloads.json") }

// ---- dlEnv ---------------------------------------------------------------------------------------------------------

func (m *dlManager) Lines() []dlLine { return m.lines() }

func (m *dlManager) Limit(ctx context.Context, line string, n int) error {
	if err := m.global.wait(ctx, n); err != nil {
		return err
	}
	m.mu.Lock()
	b := m.perLn[line]
	m.mu.Unlock()
	if b != nil {
		return b.wait(ctx, n)
	}
	return ctx.Err()
}

func (m *dlManager) HostLocked(host string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return host != "" && m.hosts[host] > time.Now().Unix()
}

func (m *dlManager) LockHost(host string) {
	if host == "" {
		return
	}
	m.mu.Lock()
	m.hosts[host] = time.Now().Add(7 * 24 * time.Hour).Unix()
	m.dirty = true
	m.mu.Unlock()
	note("info", "Downloads: %s only accepts one address per file, so downloads from it use one line", host)
}

func (m *dlManager) Changed() { m.mu.Lock(); m.dirty = true; m.mu.Unlock() }

// ---- the real lines ------------------------------------------------------------------------------------------------

var (
	dlCapMu   sync.Mutex
	dlCapAt   time.Time
	dlCapData map[string][2]float64
)

func dlLineCap(name string) float64 {
	dlCapMu.Lock()
	defer dlCapMu.Unlock()
	if time.Since(dlCapAt) > 30*time.Second {
		dlCapAt = time.Now()
		dlCapData = nil
		if b, err := os.ReadFile(capFile()); err == nil {
			_ = json.Unmarshal(b, &dlCapData)
		}
	}
	return dlCapData[name][0]
}

func dlProdLines() []dlLine {
	s := getSettings()
	allowMobile := dlMgr.config().AllowMobile
	ads := map[string]Adapter{}
	for _, a := range getAdapters(false) {
		ads[a.Name] = a
	}
	links := activeLinks(s)
	var out, mobile []dlLine
	for _, l := range links {
		a, ok := ads[l.Name]
		if !ok {
			continue
		}
		name := l.Name
		d := &net.Dialer{Timeout: 12 * time.Second, KeepAlive: 30 * time.Second, Control: bindControl(a.Index)}
		if ip := net.ParseIP(a.IP); ip != nil && ip.To4() != nil {
			d.LocalAddr = &net.TCPAddr{IP: ip}
		}
		line := dlLine{Name: name, Kind: l.Kind, Rate: dlLineCap(name),
			Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if network == "tcp" {
					network = "tcp4"
				}
				return d.DialContext(ctx, network, addr)
			},
			Alive: func() bool {
				for _, al := range activeLinks(getSettings()) {
					if al.Name == name {
						return !al.Dead
					}
				}
				return false
			}}
		if l.Kind == "mobile" {
			mobile = append(mobile, line)
		} else {
			out = append(out, line)
		}
	}
	return dlPickLines(out, mobile, allowMobile)
}

// dlPickLines: the mobile line (metered, slow) is only used for downloads when the person allowed it, or when it is the only line.
func dlPickLines(fixed, mobile []dlLine, allowMobile bool) []dlLine {
	if allowMobile || len(fixed) == 0 {
		return append(append([]dlLine{}, fixed...), mobile...)
	}
	return fixed
}

// ---- config and persistence ----------------------------------------------------------------------------------------

func (m *dlManager) config() dlConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

type dlDisk struct {
	Config dlConfig         `json:"config"`
	Items  []dlItem         `json:"items"`
	Hosts  map[string]int64 `json:"hosts"`
	Seq    int              `json:"seq"`
}

func (m *dlManager) load() {
	b, err := os.ReadFile(dlFile())
	if err != nil {
		return
	}
	var d dlDisk
	if json.Unmarshal(b, &d) != nil {
		logf("downloads.json could not be read - starting with an empty list")
		_ = os.Rename(dlFile(), dlFile()+".bad")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.Config.Dir != "" {
		m.cfg = d.Config
	}
	m.cfg = dlFixConfig(m.cfg)
	for k, v := range d.Hosts {
		if v > time.Now().Unix() {
			m.hosts[k] = v
		}
	}
	m.seq = d.Seq
	for _, it := range d.Items {
		e := &dlEntry{dlItem: it}
		switch it.State {
		case "downloading":
			e.State = "queued" // it was running when NetMerge closed: carry on
		case "queued", "paused", "done", "error":
		default:
			e.State = "error"
		}
		e.RetryAt = 0
		m.items = append(m.items, e)
	}
}

func dlFixConfig(c dlConfig) dlConfig {
	if c.Dir == "" {
		c.Dir = defaultDlDir()
	}
	if c.MaxActive < 1 || c.MaxActive > 10 {
		c.MaxActive = 3
	}
	if c.MaxPerLine < 1 || c.MaxPerLine > 16 {
		c.MaxPerLine = 8
	}
	return c
}

func (m *dlManager) save() {
	m.mu.Lock()
	d := dlDisk{Config: m.cfg, Hosts: m.hosts, Seq: m.seq}
	for _, e := range m.items {
		it := e.dlItem
		if e.task != nil {
			pr := e.prog
			it.Done = pr.Done
		}
		d.Items = append(d.Items, it)
	}
	m.dirty = false
	m.mu.Unlock()
	b, _ := json.MarshalIndent(d, "", " ")
	_ = os.MkdirAll(filepath.Dir(dlFile()), 0o755)
	_ = os.WriteFile(dlFile()+".tmp", b, 0o600)
	_ = os.Rename(dlFile()+".tmp", dlFile())
}

// ---- adding --------------------------------------------------------------------------------------------------------

func dlCleanURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 8000 {
		return "", errors.New("that is not a valid link")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", errors.New("only http and https links can be downloaded")
	}
	return u.String(), nil
}

type dlAdd struct {
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Dir    string            `json:"dir"`
	Name   string            `json:"name"`
	Lines  []string          `json:"lines"`
	Start  *bool             `json:"start"`
}

func (m *dlManager) add(a dlAdd) (*dlItem, error) {
	link, err := dlCleanURL(a.URL)
	if err != nil {
		return nil, err
	}
	if a.Dir != "" && !filepath.IsAbs(a.Dir) {
		return nil, errors.New("choose a full folder path, for example C:\\Downloads")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	e := &dlEntry{}
	e.ID = fmt.Sprintf("d%d-%d", time.Now().Unix(), m.seq)
	e.URL, e.Header, e.Dir, e.Lines, e.Added = link, dlCleanHeader(a.Header), a.Dir, a.Lines, time.Now().Unix()
	e.Name = dlPickName("", link)
	if a.Name != "" {
		e.Name = dlSafeName(a.Name)
	}
	e.Category = dlCategory(e.Name)
	e.State = "queued"
	if a.Start != nil && !*a.Start {
		e.State = "paused"
	}
	m.items = append(m.items, e)
	m.dirty = true
	it := e.dlItem
	it.Header = nil
	note("info", "Download added: %s", e.Name)
	return &it, nil
}

func dlCategory(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".msi", ".msix", ".apk", ".dmg", ".iso", ".deb", ".rpm", ".appx":
		return "Programs"
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".cab":
		return "Archives"
	case ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".webm", ".flv", ".m4v", ".ts":
		return "Videos"
	case ".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a", ".opus":
		return "Music"
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".epub", ".csv", ".odt", ".rtf":
		return "Documents"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg", ".tif", ".tiff", ".heic":
		return "Images"
	}
	return "Other"
}

// pickPath decides where the file goes once the server told us its name. Also used again on retry, so an item keeps its path.
func (m *dlManager) pickPath(e *dlEntry, info dlInfo) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.Path != "" {
		return e.Path, nil
	}
	name := e.Name
	if info.Name != "" && info.Name != "download" {
		name = info.Name
	}
	if e.nameFixed() {
		name = e.Name
	}
	dir := e.Dir
	if dir == "" {
		dir = m.cfg.Dir
		if m.cfg.UseCategories {
			dir = filepath.Join(dir, dlCategory(name))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create the folder %s: %v", dir, err)
	}
	taken := func(p string) bool {
		if _, err := os.Stat(p); err == nil {
			return true
		}
		if _, err := os.Stat(dlPartPath(p)); err == nil {
			return true
		}
		for _, o := range m.items {
			if o != e && o.Path == p {
				return true
			}
		}
		return false
	}
	p := dlUniquePath(dir, name, taken)
	e.Path, e.Name = p, filepath.Base(p)
	e.Category = dlCategory(e.Name)
	m.dirty = true
	return p, nil
}

func (e *dlEntry) nameFixed() bool { return false }

// ---- running -------------------------------------------------------------------------------------------------------

func (m *dlManager) startLocked(e *dlEntry) {
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel, e.running, e.wantPause = cancel, true, false
	e.State, e.Err = "downloading", ""
	e.doneCh = make(chan struct{})
	spec := dlSpec{URL: e.URL, Header: e.Header, Path: e.Path, Lines: e.Lines, MaxPerLine: m.cfg.MaxPerLine}
	spec.Pick = func(info dlInfo) (string, error) { return m.pickPath(e, info) }
	spec.OnInfo = func(info dlInfo, path string) {
		m.mu.Lock()
		e.Size = info.Size
		if e.Path == "" {
			e.Path = path
		}
		m.dirty = true
		m.mu.Unlock()
	}
	t := newDlTask(spec, m)
	e.task = t
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		err := t.Run(ctx)
		m.finishRun(e, t, err)
	}()
}

func (m *dlManager) finishRun(e *dlEntry, t *dlTask, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr := t.Progress()
	e.running = false
	e.prog = pr
	e.task = nil
	e.Done = pr.Done
	if e.Bytes == nil {
		e.Bytes = map[string]int64{}
	}
	for _, l := range pr.Lines {
		e.Bytes[l.Name] += l.Bytes
	}
	m.dirty = true
	close(e.doneCh)
	switch {
	case err == nil:
		e.State, e.Err, e.Finished = "done", "", time.Now().Unix()
		if p := t.FinalPath(); p != "" {
			e.Path = p
			e.Name = filepath.Base(p)
		}
		if e.Size < 0 || pr.Done > e.Size {
			e.Size = pr.Done
		}
		e.Done = e.Size
		e.Note = pr.Note
		note("good", "Download finished: %s", e.Name)
	case e.wantPause:
		e.State = "paused"
	case m.quit || errors.Is(err, context.Canceled):
		e.State = "queued" // NetMerge is closing: carry on at the next start
	default:
		msg, retry := dlFriendly(err)
		e.Err = msg
		if retry && e.Retries < 6 {
			e.Retries++
			e.State = "queued"
			e.RetryAt = time.Now().Add(time.Duration(5<<uint(e.Retries-1)) * time.Second).Unix()
			e.Note = "Trying again soon"
		} else if retry && e.Retries >= 6 {
			e.State = "error"
			note("bad", "Download failed: %s (%s)", e.Name, msg)
		} else {
			e.State = "error"
			note("bad", "Download failed: %s (%s)", e.Name, msg)
		}
	}
}

// dlFriendly turns an error into words a person understands. The bool says whether trying again could help.
func dlFriendly(err error) (string, bool) {
	var he dlHTTPErr
	switch {
	case errors.Is(err, errDlChanged):
		return "The file on the website changed while it was downloading. Start it again.", false
	case errors.As(err, &he):
		switch he.Code {
		case 401, 403:
			return "The website refused the download (it may need you to sign in, or the link expired). Open the page in your browser and add the link again.", false
		case 404, 410:
			return "The file was not found on the website. The link may be old.", false
		case 429, 503:
			return "The website is busy or limits downloads. NetMerge will try again.", true
		}
		return he.Error(), he.Code >= 500
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "no connection is available"):
		return "Waiting for an internet connection.", true
	case strings.Contains(s, "not enough space"), strings.Contains(s, "no space left"), strings.Contains(s, "disk is full"), strings.Contains(s, "There is not enough space"):
		return "The disk is full. Free some space and press Resume.", false
	case strings.Contains(s, "cannot write"), strings.Contains(s, "cannot create the folder"), strings.Contains(s, "Access is denied"), strings.Contains(s, "permission denied"):
		return "NetMerge cannot write to the download folder. Choose another folder in the Downloads settings.", false
	case strings.Contains(s, "only http and https"), strings.Contains(s, "refused: the link leads"):
		return s, false
	}
	return "The connection failed: " + dlFmtErr(err), true
}

// loop starts queued downloads when there is room, refreshes progress and saves.
// startLoop runs the queue in the background until shutdown.
func (m *dlManager) startLoop() {
	m.loopWG.Add(1)
	go func() { defer m.loopWG.Done(); m.loop() }()
}

func (m *dlManager) loop() {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	n := 0
	for range tick.C {
		n++
		m.mu.Lock()
		if m.quit {
			m.mu.Unlock()
			return
		}
		active := 0
		for _, e := range m.items {
			if e.running {
				active++
				if e.task != nil {
					e.prog = e.task.Progress()
				}
			}
		}
		now := time.Now().Unix()
		for _, e := range m.items {
			if active >= m.cfg.MaxActive {
				break
			}
			if e.State == "queued" && !e.running && e.RetryAt <= now {
				m.startLocked(e)
				active++
			}
		}
		save := m.dirty && n%4 == 0
		m.mu.Unlock()
		if save {
			m.save()
		}
	}
}

// ---- actions -------------------------------------------------------------------------------------------------------

func (m *dlManager) find(id string) *dlEntry {
	for _, e := range m.items {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (m *dlManager) pause(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.find(id)
	if e == nil {
		return errors.New("download not found")
	}
	switch {
	case e.running:
		e.wantPause = true
		e.cancel()
	case e.State == "queued" || e.State == "error":
		e.State = "paused"
	}
	m.dirty = true
	return nil
}

func (m *dlManager) resume(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.find(id)
	if e == nil {
		return errors.New("download not found")
	}
	if e.State == "paused" || e.State == "error" {
		e.State, e.Err, e.Retries, e.RetryAt = "queued", "", 0, 0
		if e.running { // a pause is still finishing: it will start from the loop
			e.State = "paused"
			return errors.New("still pausing, try again in a second")
		}
	}
	m.dirty = true
	return nil
}

// remove takes a download off the list. deleteFiles also deletes the finished file or the partial one.
func (m *dlManager) remove(id string, deleteFiles bool) error {
	m.mu.Lock()
	e := m.find(id)
	if e == nil {
		m.mu.Unlock()
		return errors.New("download not found")
	}
	var wait chan struct{}
	if e.running {
		e.wantPause = true
		e.cancel()
		wait = e.doneCh
	}
	m.mu.Unlock()
	if wait != nil {
		select {
		case <-wait:
		case <-time.After(5 * time.Second):
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, x := range m.items {
		if x == e {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}
	if e.Path != "" {
		if e.State != "done" { // unfinished: the partial files are of no use any more
			_ = os.Remove(dlPartPath(e.Path))
			_ = os.Remove(dlStatePath(e.Path))
		} else if deleteFiles {
			_ = os.Remove(e.Path)
		}
	}
	m.dirty = true
	return nil
}

func (m *dlManager) clearFinished() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keep []*dlEntry
	n := 0
	for _, e := range m.items {
		if e.State == "done" {
			n++
			continue
		}
		keep = append(keep, e)
	}
	m.items = keep
	m.dirty = true
	return n
}

func (m *dlManager) setConfig(c dlConfig) error {
	c = dlFixConfig(c)
	if !filepath.IsAbs(c.Dir) {
		return errors.New("choose a full folder path, for example C:\\Downloads")
	}
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return fmt.Errorf("cannot create that folder: %v", err)
	}
	f, err := os.CreateTemp(c.Dir, ".nm-test-*")
	if err != nil {
		return fmt.Errorf("NetMerge cannot write into that folder: %v", err)
	}
	f.Close()
	_ = os.Remove(f.Name())
	m.mu.Lock()
	m.cfg = c
	m.dirty = true
	m.mu.Unlock()
	return nil
}

func (m *dlManager) views() []dlView {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]dlView, 0, len(m.items))
	for i, e := range m.items {
		v := dlView{dlItem: e.dlItem, Order: i}
		v.dlItem.Header = nil
		if e.running && e.task != nil {
			pr := e.prog
			v.Done, v.Speed, v.LineStat = pr.Done, pr.Speed, pr.Lines
			if pr.Note != "" {
				v.Note = pr.Note
			}
			if pr.Total > 0 {
				v.Size = pr.Total
			}
		}
		out = append(out, v)
	}
	return out
}

func (m *dlManager) shutdown() {
	m.mu.Lock()
	m.quit = true
	for _, e := range m.items {
		if e.running {
			e.cancel()
		}
	}
	m.mu.Unlock()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
	}
	m.loopWG.Wait()
	m.save()
}

func (m *dlManager) summary() (active int, speed float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.items {
		if e.running {
			active++
			speed += e.prog.Speed
		}
	}
	return
}

var _ = sort.Strings
