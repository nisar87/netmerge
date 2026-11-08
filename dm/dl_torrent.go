package dm

// Torrents in the download list: magnet links, .torrent links and .torrent files all become an item of kind "torrent".

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dlRunner is what the list needs from a running download, a file (dlTask) or a torrent (btTask).
type dlRunner interface {
	Run(ctx context.Context) error
	Progress() dlProgress
	FinalPath() string
	SetRate(bytesPerSec float64)
}

func (t *dlTask) SetRate(r float64) { t.bucket.setRate(r) }

type dlTFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (e *dlEntry) cat(name string) string {
	if e.Kind == "torrent" {
		return "Torrents"
	}
	return dlCategory(name)
}

func btDir() string { return filepath.Join(hRoot(), "data", "torrents") }

func btCopyPath(hash string) string { return filepath.Join(btDir(), hash+".torrent") }

func btLoadCopy(hash string) *btMeta {
	b, err := os.ReadFile(btCopyPath(hash))
	if err != nil {
		return nil
	}
	m, err := btParseTorrent(b)
	if err != nil {
		return nil
	}
	return m
}

func btSaveCopy(m *btMeta) {
	_ = os.MkdirAll(btDir(), 0o755)
	_ = dlWriteFileAtomicMode(btCopyPath(fmt.Sprintf("%x", m.InfoHash)), btTorrentFile(m), 0o600)
}

// btHashOfLink finds the torrent hash inside a magnet link ("" when it is not one).
func btHashOfLink(link string) string {
	if !strings.HasPrefix(strings.ToLower(link), "magnet:") {
		return ""
	}
	if mg, err := btParseMagnet(link); err == nil {
		return fmt.Sprintf("%x", mg.Hash)
	}
	return ""
}

func (m *dlManager) addTorrent(a dlAdd) (*dlItem, error) {
	raw := strings.TrimSpace(a.URL)
	var link, name string
	var meta *btMeta
	switch {
	case a.Torrent != "":
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(a.Torrent))
		if err != nil || len(b) > 16<<20 {
			return nil, errors.New("that is not a usable .torrent file")
		}
		meta, err = btParseTorrent(b)
		if err != nil {
			return nil, err
		}
		btSaveCopy(meta)
		link, name = btMagnetLink(meta.InfoHash, meta.Name, meta.Trackers), meta.Name
	case strings.HasPrefix(strings.ToLower(raw), "magnet:"):
		mg, err := btParseMagnet(raw)
		if err != nil {
			return nil, err
		}
		link, name = btMagnetLink(mg.Hash, mg.Name, mg.Trackers), mg.Name
		if name == "" {
			name = fmt.Sprintf("Torrent %x", mg.Hash[:4])
		}
		if len(mg.Peers) > 0 { // hints are kept in the link
			link += "&x.pe=" + strings.Join(mg.Peers, "&x.pe=")
		}
	case btLooksLikeTorrentURL(raw):
		l, err := dlCleanURL(raw)
		if err != nil {
			return nil, err
		}
		l, auth := dlSplitAuth(l)
		if auth != "" {
			h := map[string]string{}
			for k, v := range a.Header {
				h[k] = v
			}
			h["Authorization"] = auth
			a.Header = h
		}
		link = l
		name = dlPickName("", l)
		name = strings.TrimSuffix(name, filepath.Ext(name))
	default:
		return nil, nil
	}
	if a.Dir != "" {
		d, err := dlCleanDir(a.Dir)
		if err != nil {
			return nil, err
		}
		a.Dir = d
	}
	if a.Limit < 0 || a.Limit > 1e12 {
		a.Limit = 0
	}
	hash := btHashOfLink(link)
	m.mu.Lock()
	defer m.mu.Unlock()
	if a.Queue != "" && m.cfg.queue(a.Queue) == nil {
		return nil, errors.New("that queue does not exist")
	}
	if len(m.items) >= 5000 {
		return nil, errors.New("the list is full (5000 downloads): clear the finished ones first")
	}
	if !a.Force {
		for _, o := range m.items {
			if o.URL == link || (hash != "" && btHashOfLink(o.URL) == hash) {
				return nil, errDlDuplicate
			}
		}
	}
	m.seq++
	e := &dlEntry{}
	e.Kind, e.Limit, e.Web = "torrent", a.Limit, a.FromWeb
	if a.StartAt > time.Now().Unix() {
		e.StartAt = a.StartAt
	}
	e.ID = fmt.Sprintf("d%d-%d", time.Now().Unix(), m.seq)
	e.URL, e.Header, e.Dir, e.Lines, e.Added = link, dlCleanHeader(a.Header), a.Dir, a.Lines, time.Now().Unix()
	e.Name = dlSafeName(name)
	if a.Name != "" {
		e.Name = dlSafeName(a.Name)
	}
	e.Category = "Torrents"
	e.State = "queued"
	if a.Start != nil && !*a.Start {
		e.State = "paused"
	}
	e.Queue = a.Queue
	if meta != nil {
		e.Size = meta.Total
		for _, f := range meta.Files {
			e.Files = append(e.Files, dlTFile{Name: f.name(), Size: f.Length})
		}
	}
	m.elog(e, "Torrent added")
	m.items = append(m.items, e)
	m.dirty = true
	it := e.dlItem
	it.Header = nil
	hNote("info", "Torrent added: %s", e.Name)
	return &it, nil
}

func (m *dlManager) newTorrentTask(e *dlEntry) *btTask {
	spec := btSpec{Header: e.Header, Lines: e.Lines, Selected: e.Select}
	hash := btHashOfLink(e.URL)
	switch {
	case hash != "" && btLoadCopy(hash) != nil:
		spec.Meta = btLoadCopy(hash)
	case hash != "":
		mg, _ := btParseMagnet(e.URL)
		spec.Magnet = mg
	default:
		spec.TorrentURL = e.URL
	}
	spec.MaxPeers = 40 * max(1, len(m.lines()))
	if spec.MaxPeers > 150 {
		spec.MaxPeers = 150
	}
	spec.Pick = func(name string) (string, error) { return m.pickPath(e, dlInfo{Name: name}) }
	spec.SaveMeta = btSaveCopy
	spec.OnInfo = func(mt *btMeta, path string) {
		m.mu.Lock()
		e.Size = mt.Total
		if e.Path == "" {
			e.Path = path
		}
		files := make([]dlTFile, 0, len(mt.Files))
		for _, f := range mt.Files {
			files = append(files, dlTFile{Name: f.name(), Size: f.Length})
		}
		e.Files = files
		m.dirty = true
		m.mu.Unlock()
	}
	return newBtTask(spec, m)
}

// dropTorrentFiles is called (with the lock held) when a torrent is removed from the list.
func (m *dlManager) dropTorrentFiles(e *dlEntry, deleteFiles bool) {
	if e.Path != "" {
		_ = os.Remove(btStatePath(e.Path))
		if e.State != "done" || deleteFiles {
			_ = os.RemoveAll(e.Path)
		}
	}
	hash := btHashOfLink(e.URL)
	if hash == "" {
		return
	}
	for _, o := range m.items {
		if o != e && btHashOfLink(o.URL) == hash {
			return
		}
	}
	_ = os.Remove(btCopyPath(hash))
}

// setSelect chooses which files of a torrent to download. A running torrent is restarted so it takes effect.
func (m *dlManager) setSelect(id string, sel []bool) error {
	m.mu.Lock()
	e := m.find(id)
	if e == nil || e.Kind != "torrent" {
		m.mu.Unlock()
		return errors.New("torrent not found")
	}
	if len(e.Files) == 0 {
		m.mu.Unlock()
		return errors.New("the files of this torrent are not known yet")
	}
	if len(sel) != len(e.Files) {
		m.mu.Unlock()
		return errors.New("the list of files does not match")
	}
	any := false
	for _, s := range sel {
		any = any || s
	}
	if !any {
		m.mu.Unlock()
		return errors.New("choose at least one file")
	}
	all := true
	for _, s := range sel {
		all = all && s
	}
	if all {
		e.Select = nil
	} else {
		e.Select = append([]bool(nil), sel...)
	}
	m.dirty = true
	running := e.running
	m.mu.Unlock()
	if running {
		if err := m.pause(id); err != nil {
			return err
		}
		m.mu.Lock()
		wait := func() chan struct{} {
			if e := m.find(id); e != nil {
				return e.doneCh
			}
			return nil
		}()
		m.mu.Unlock()
		if wait != nil {
			select {
			case <-wait:
			case <-time.After(5 * time.Second):
			}
		}
		return m.resume(id)
	}
	return nil
}
