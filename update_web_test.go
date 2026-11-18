package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// When GitHub's API refuses (403, rate limit), the check falls back to the release page and still finds the
// newest version with its download and checksum addresses.
func TestUpdateCheckFallsBackToReleasePage(t *testing.T) {
	sum := strings.Repeat("ab", 32)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "rate limited", 403) })
	mux.HandleFunc("/"+updateRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+updateRepo+"/releases/tag/v99.0.0", http.StatusFound)
	})
	mux.HandleFunc("/"+updateRepo+"/releases/download/v99.0.0/NetMerge.exe.sha256", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sum + "  NetMerge.exe\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	oldAPI, oldWeb := updateAPI, updateWeb
	updateAPI, updateWeb = srv.URL, srv.URL
	defer func() { updateAPI, updateWeb = oldAPI, oldWeb }()

	checkUpdate(true)
	updMu.Lock()
	defer updMu.Unlock()
	if upd.Error != "" || upd.Latest != "99.0.0" || !upd.Available {
		t.Fatalf("check result: latest %q available %v error %q", upd.Latest, upd.Available, upd.Error)
	}
	if upd.sha != sum || !strings.HasSuffix(upd.asset, "/releases/download/v99.0.0/NetMerge.exe") {
		t.Fatalf("asset %q sha %q", upd.asset, upd.sha)
	}
}

// A second "Update now" while the first is still downloading or installing is answered as fine, not with
// "there is no update to install".
func TestSecondInstallWhileUnderWayIsNotAnError(t *testing.T) {
	updMu.Lock()
	saved := upd
	updMu.Unlock()
	defer func() { updMu.Lock(); upd = saved; updMu.Unlock() }()
	for _, st := range []string{"downloading", "installing"} {
		updMu.Lock()
		upd = UpdateState{Status: st, Available: false}
		updMu.Unlock()
		if err := installUpdate(); err != nil {
			t.Fatalf("status %s: %v", st, err)
		}
		if got := updateSnapshot().Status; got != st {
			t.Fatalf("status changed from %s to %s", st, got)
		}
	}
	updMu.Lock()
	upd = UpdateState{Status: "idle", Available: false}
	updMu.Unlock()
	if err := installUpdate(); err == nil {
		t.Fatal("with no update available, install should still say so")
	}
}
