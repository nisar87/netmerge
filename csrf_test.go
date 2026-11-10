package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Any website can make the browser send a GET to 127.0.0.1:9196 (an image, a link). Such a request must never
// change anything: turn merging off, quit, start a speed test or change settings.
func TestGETCannotChangeAnything(t *testing.T) {
	h := action(func(w http.ResponseWriter, r *http.Request) { t.Fatalf("%s %s reached the handler", r.Method, r.URL) })
	for _, req := range []*http.Request{
		httptest.NewRequest("GET", "http://127.0.0.1:9196/api/quit", nil),
		httptest.NewRequest("HEAD", "http://127.0.0.1:9196/api/power", nil),
		func() *http.Request { // even with the header, an action needs POST
			r := httptest.NewRequest("GET", "http://127.0.0.1:9196/api/speedtest", nil)
			r.Header.Set("X-NetMerge", "1")
			return r
		}(),
		httptest.NewRequest("POST", "http://127.0.0.1:9196/api/power", nil), // POST without the header (a form)
	} {
		w := httptest.NewRecorder()
		h(w, req)
		if w.Code < 400 {
			t.Errorf("%s %s answered %d", req.Method, req.URL, w.Code)
		}
	}
	ok := false
	good := action(func(w http.ResponseWriter, r *http.Request) { ok = true })
	r := httptest.NewRequest("POST", "http://127.0.0.1:9196/api/power", nil)
	r.Header.Set("X-NetMerge", "1")
	r.Header.Set("Origin", "http://127.0.0.1:9196")
	good(httptest.NewRecorder(), r)
	if !ok {
		t.Fatal("the page's own POST was refused")
	}
}
