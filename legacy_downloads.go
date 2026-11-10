package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NetMerge used to include a download manager. It is now a program of its own, Pullwell. NetMerge no longer runs
// downloads, but a person who used them keeps the list (data\downloads.json) and the files on disk: nothing is
// deleted. The list can be saved here in the format Pullwell imports ("netmerge-downloads").

const pullwellURL = "https://github.com/nisar87/pullwell"

func legacyDownloadFile() string { return filepath.Join(rootDir, "data", "downloads.json") }

type legacyDownloadItem struct {
	URL      string   `json:"url"`
	Name     string   `json:"name,omitempty"`
	Dir      string   `json:"dir,omitempty"`
	Queue    string   `json:"queue,omitempty"`
	Mirrors  []string `json:"mirrors,omitempty"`
	Hash     string   `json:"hash,omitempty"`
	Finished bool     `json:"finished,omitempty"`
}

type legacyDownloadList struct {
	App     string               `json:"app"`
	Version int                  `json:"version"`
	Queues  []json.RawMessage    `json:"queues,omitempty"`
	Items   []legacyDownloadItem `json:"items"`
}

// readLegacyDownloads turns the old list into the export format. ok is false when there is no usable list.
func readLegacyDownloads() (legacyDownloadList, bool) {
	out := legacyDownloadList{App: "netmerge-downloads", Version: 1}
	b, err := os.ReadFile(legacyDownloadFile())
	if err != nil {
		return out, false
	}
	var disk struct {
		Config struct {
			Queues []json.RawMessage `json:"queues"`
		} `json:"config"`
		Items []struct {
			URL     string   `json:"url"`
			Name    string   `json:"name"`
			Dir     string   `json:"dir"`
			Queue   string   `json:"queue"`
			Mirrors []string `json:"mirrors"`
			Hash    string   `json:"hash"`
			State   string   `json:"state"`
		} `json:"items"`
	}
	if json.Unmarshal(b, &disk) != nil {
		return out, false
	}
	out.Queues = disk.Config.Queues
	for _, it := range disk.Items {
		if it.URL == "" {
			continue
		}
		out.Items = append(out.Items, legacyDownloadItem{URL: it.URL, Name: it.Name, Dir: it.Dir, Queue: it.Queue,
			Mirrors: it.Mirrors, Hash: it.Hash, Finished: it.State == "done"})
	}
	return out, len(out.Items) > 0
}

// legacyDownloadCount: how many downloads the old list holds (0 = nothing to move, the page shows no note).
// The page asks every second, so the file is read again only when it changes.
func legacyDownloadCount() int {
	st, err := os.Stat(legacyDownloadFile())
	if err != nil {
		return 0
	}
	legacyMu.Lock()
	defer legacyMu.Unlock()
	if !st.ModTime().Equal(legacyAt) {
		legacyAt, legacyN = st.ModTime(), 0
		if l, ok := readLegacyDownloads(); ok {
			legacyN = len(l.Items)
		}
	}
	return legacyN
}

var (
	legacyMu sync.Mutex
	legacyAt time.Time
	legacyN  int
)

func registerLegacyDownloadRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/legacy-downloads", guard(func(w http.ResponseWriter, r *http.Request) {
		l, ok := readLegacyDownloads()
		if !ok {
			writeErr(w, 404, "There is no download list from an earlier NetMerge version")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="netmerge-download-list-`+time.Now().Format("2006-01-02")+`.json"`)
		b, _ := json.MarshalIndent(l, "", "  ")
		_, _ = w.Write(b)
	}))
}
