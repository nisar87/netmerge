package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 24-hour speed history: one point per minute per connection (average Mbps, down and up), kept in memory and
// saved to data\history.json every few minutes so a restart does not lose the picture.
type histPoint struct {
	T int64              `json:"t"` // unix seconds
	L map[string]float64 `json:"d"` // download Mbps per connection name
	U map[string]float64 `json:"u"` // upload Mbps per connection name
}

const histMax = 1440

var (
	histMu   sync.Mutex
	histData []histPoint
)

func historyPath() string { return filepath.Join(rootDir, "data", "history.json") }

func historyLoad() {
	b, err := os.ReadFile(historyPath())
	if err != nil {
		return
	}
	var h []histPoint
	if json.Unmarshal(b, &h) != nil {
		return
	}
	cut := time.Now().Add(-24 * time.Hour).Unix()
	histMu.Lock()
	for _, p := range h {
		if p.T >= cut {
			histData = append(histData, p)
		}
	}
	histMu.Unlock()
}

func historySave() {
	histMu.Lock()
	b, _ := json.Marshal(histData)
	histMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(historyPath()), 0o755)
	tmp := historyPath() + ".tmp"
	if logErr("save the speed history", os.WriteFile(tmp, b, 0o644)) == nil {
		logErr("save the speed history", os.Rename(tmp, historyPath()))
	}
}

// histAppend adds one minute (trimming to 24 h).
func histAppend(p histPoint) {
	histMu.Lock()
	histData = append(histData, p)
	if len(histData) > histMax {
		histData = histData[len(histData)-histMax:]
	}
	histMu.Unlock()
}

func historyLoop() {
	historyLoad()
	sumD, sumU := map[string]float64{}, map[string]float64{}
	n, saved := 0, 0
	t := time.NewTicker(5 * time.Second)
	for range t.C {
		statsMu.Lock()
		for name, tr := range tracks {
			sumD[name] += tr.rx
			sumU[name] += tr.tx
		}
		statsMu.Unlock()
		n++
		if n < 12 {
			continue
		}
		p := histPoint{T: time.Now().Unix(), L: map[string]float64{}, U: map[string]float64{}}
		for name := range sumD {
			p.L[name] = round1(sumD[name] / float64(n) * 8 / 1e6)
			p.U[name] = round1(sumU[name] / float64(n) * 8 / 1e6)
		}
		histAppend(p)
		sumD, sumU, n = map[string]float64{}, map[string]float64{}, 0
		if saved++; saved >= 5 {
			saved = 0
			historySave()
		}
	}
}

func registerHistoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/history", guard(func(w http.ResponseWriter, r *http.Request) {
		histMu.Lock()
		out := append([]histPoint(nil), histData...)
		histMu.Unlock()
		writeJSON(w, map[string]any{"points": out})
	}))
}
