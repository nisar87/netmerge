package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The list of the removed download manager can be saved in the format Pullwell imports, and nothing is shown
// to people who never used it.
func TestLegacyDownloadListForPullwell(t *testing.T) {
	old := rootDir
	rootDir = t.TempDir()
	defer func() { rootDir = old }()
	if n := legacyDownloadCount(); n != 0 {
		t.Fatalf("no list, but count %d", n)
	}
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	disk := `{"config":{"dir":"D:\\dl","queues":[{"id":"q1","name":"Night","maxActive":1}]},
	  "items":[{"id":"d1","url":"https://example.com/a.iso","name":"a.iso","dir":"D:\\iso","queue":"q1","state":"done","hash":"sha256:ab"},
	           {"id":"d2","url":"https://example.com/b.zip","name":"b.zip","state":"paused","header":{"Cookie":"secret"}},
	           {"id":"d3","url":"","state":"error"}],
	  "hosts":{},"seq":3}`
	if err := os.WriteFile(legacyDownloadFile(), []byte(disk), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := legacyDownloadCount(); n != 2 {
		t.Fatalf("count %d, want 2", n)
	}
	mux := http.NewServeMux()
	registerLegacyDownloadRoutes(mux)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://127.0.0.1:9196/api/legacy-downloads", nil)
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var got legacyDownloadList
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.App != "netmerge-downloads" || got.Version != 1 || len(got.Queues) != 1 || len(got.Items) != 2 {
		t.Fatalf("wrong file: %+v", got)
	}
	a, b := got.Items[0], got.Items[1]
	if a.URL != "https://example.com/a.iso" || !a.Finished || a.Dir != `D:\iso` || a.Queue != "q1" || a.Hash != "sha256:ab" {
		t.Fatalf("first item: %+v", a)
	}
	if b.Finished || b.Name != "b.zip" {
		t.Fatalf("second item: %+v", b)
	}
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("a cookie from the old list ended up in the file")
	}
	if _, err := os.Stat(legacyDownloadFile()); err != nil {
		t.Fatal("the old list must stay where it is")
	}
}
