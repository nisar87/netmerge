package dm

// BitTorrent: putting the pieces on the disk, and remembering which ones we have.

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type btStore struct {
	meta  *btMeta
	root  string // single file: the file itself; several files: the folder
	mu    sync.Mutex
	files []*os.File
}

func btStatePath(root string) string { return root + ".nmbt" }

func (s *btStore) pathOf(i int) string {
	if !s.meta.Multi {
		return s.root
	}
	return filepath.Join(append([]string{s.root}, s.meta.Files[i].Path...)...)
}

// prepare creates the folders and (sparse) files, so a full disk shows up at the start and not at the end.
func (s *btStore) prepare() error {
	s.files = make([]*os.File, len(s.meta.Files))
	for i, f := range s.meta.Files {
		p := s.pathOf(i)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return fmt.Errorf("cannot create the folder %s: %v", filepath.Dir(p), err)
		}
		fh, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("cannot write %s: %v", p, err)
		}
		if st, err := fh.Stat(); err == nil && st.Size() != f.Length {
			if err := fh.Truncate(f.Length); err != nil {
				fh.Close()
				return fmt.Errorf("cannot write %s: %v", p, err)
			}
		}
		s.files[i] = fh
	}
	return nil
}

func (s *btStore) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.files {
		if f != nil {
			f.Close()
		}
	}
	s.files = nil
}

// span calls fn for every file piece that the range [off, off+n) of the torrent covers.
func (s *btStore) span(off int64, n int, fn func(f *os.File, fo int64, a, b int) error) error {
	s.mu.Lock()
	files := s.files
	s.mu.Unlock()
	if files == nil {
		return errors.New("closed")
	}
	pos := 0
	for i, f := range s.meta.Files {
		if n-pos <= 0 {
			break
		}
		end := f.off + f.Length
		cur := off + int64(pos)
		if cur >= end || f.Length == 0 {
			continue
		}
		fo := cur - f.off
		take := int(end - cur)
		if take > n-pos {
			take = n - pos
		}
		if err := fn(files[i], fo, pos, pos+take); err != nil {
			return err
		}
		pos += take
	}
	return nil
}

func (s *btStore) writeAt(off int64, p []byte) error {
	return s.span(off, len(p), func(f *os.File, fo int64, a, b int) error {
		_, err := f.WriteAt(p[a:b], fo)
		return err
	})
}

func (s *btStore) readAt(off int64, p []byte) error {
	return s.span(off, len(p), func(f *os.File, fo int64, a, b int) error {
		_, err := f.ReadAt(p[a:b], fo)
		return err
	})
}

func (s *btStore) sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.files {
		if f != nil {
			_ = f.Sync()
		}
	}
}

// check hashes piece i as it is on the disk.
func (s *btStore) check(i int, buf []byte) bool {
	n := int(s.meta.pieceSize(i))
	if len(buf) < n {
		return false
	}
	if err := s.readAt(int64(i)*s.meta.PieceLen, buf[:n]); err != nil {
		return false
	}
	h := sha1.Sum(buf[:n])
	return string(h[:]) == string(s.meta.hashOf(i))
}

type btState struct {
	V    int    `json:"v"`
	Hash string `json:"hash"`
	Have string `json:"have"`
}

func btSaveState(root string, m *btMeta, have []bool) {
	bits := make([]byte, (len(have)+7)/8)
	for i, h := range have {
		if h {
			bits[i/8] |= 0x80 >> (i % 8)
		}
	}
	b, _ := json.Marshal(btState{V: 1, Hash: fmt.Sprintf("%x", m.InfoHash), Have: base64.StdEncoding.EncodeToString(bits)})
	_ = dlWriteFileAtomicMode(btStatePath(root), b, 0o600)
}

func btLoadState(root string, m *btMeta) []bool {
	b, err := os.ReadFile(btStatePath(root))
	if err != nil {
		return nil
	}
	var st btState
	if json.Unmarshal(b, &st) != nil || st.V != 1 || st.Hash != fmt.Sprintf("%x", m.InfoHash) {
		return nil
	}
	bits, err := base64.StdEncoding.DecodeString(st.Have)
	if err != nil || len(bits) != (m.pieces()+7)/8 {
		return nil
	}
	have := make([]bool, m.pieces())
	for i := range have {
		have[i] = bits[i/8]&(0x80>>(i%8)) != 0
	}
	return have
}
