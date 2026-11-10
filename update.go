package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Updates. NetMerge looks at the newest release on GitHub, tells you when there is one, and installs it on
// request: it downloads NetMerge.exe, checks it against the SHA-256 fingerprint published with the release,
// and then swaps it in and restarts (the old file is kept as NetMerge-previous.exe, which ROLLBACK.bat restores).

// Set at build time:  -ldflags "-X main.appVersion=2.2.0 -X main.updateRepo=owner/netmerge"
var (
	appVersion = "3.13.1"
	updateRepo = "nisar87/netmerge" // "owner/repo" of the GitHub project (the release build sets it too)
)

var updateAPI = "https://api.github.com" // overridden in tests by NETMERGE_UPDATE_API

type UpdateState struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	Page      string `json:"page"`      // release page
	Available bool   `json:"available"` // a newer version exists
	Checked   int64  `json:"checked"`   // unix seconds of the last successful check
	Error     string `json:"error"`
	Status    string `json:"status"`   // "idle" | "checking" | "downloading" | "installing"
	Progress  int    `json:"progress"` // percent while downloading
	Repo      string `json:"repo"`
	Auto      bool   `json:"auto"`
	EngineDL  string `json:"engineDl"` // "" | "downloading" | "error"
	EnginePct int    `json:"enginePct"`
	EngineErr string `json:"engineErr"`

	asset, sha string
}

var (
	updMu  sync.Mutex
	upd    = UpdateState{Status: "idle"}
	notifd string
)

func updateSnapshot() UpdateState {
	updMu.Lock()
	defer updMu.Unlock()
	u := upd
	u.Current, u.Repo, u.Auto = appVersion, updateRepo, !getSettings().NoUpdateCheck
	u.EngineDL, u.EnginePct, u.EngineErr = engineDownloadState()
	return u
}

// versionParts turns "v2.10.1" into [2 10 1].
func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// newerVersion reports whether a is newer than b.
func newerVersion(a, b string) bool {
	x, y := versionParts(a), versionParts(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			return p > q
		}
	}
	return false
}

type ghRelease struct {
	Tag    string `json:"tag_name"`
	Name   string `json:"name"`
	Body   string `json:"body"`
	Page   string `json:"html_url"`
	Pre    bool   `json:"prerelease"`
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

func httpGet(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NetMerge/"+appVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	return (&http.Client{Timeout: 0}).Do(req)
}

// checkUpdate asks GitHub for the newest release. It never changes anything on disk.
func checkUpdate(manual bool) {
	updMu.Lock()
	if upd.Status == "checking" || upd.Status == "downloading" || upd.Status == "installing" {
		updMu.Unlock()
		return
	}
	if updateRepo == "" || strings.Contains(updateRepo, "REPLACE") {
		upd.Error = "Updates are not set up in this build."
		updMu.Unlock()
		return
	}
	upd.Status, upd.Error = "checking", ""
	updMu.Unlock()

	fail := func(msg string) {
		updMu.Lock()
		upd.Status = "idle"
		if manual {
			upd.Error = msg
		}
		updMu.Unlock()
		logf("update check: %s", msg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	resp, err := httpGet(ctx, updateAPI+"/repos/"+updateRepo+"/releases/latest")
	if err != nil {
		fail("Could not reach GitHub.")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		fail("No release has been published yet.")
		return
	}
	if resp.StatusCode != 200 {
		fail(fmt.Sprintf("GitHub answered HTTP %d.", resp.StatusCode))
		return
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil || rel.Tag == "" {
		fail("Could not read the release information.")
		return
	}
	var asset, sha, shaURL string
	for _, a := range rel.Assets {
		switch {
		case strings.EqualFold(a.Name, "NetMerge.exe"):
			asset = a.URL
			sha = strings.TrimPrefix(strings.ToLower(a.Digest), "sha256:")
		case strings.EqualFold(a.Name, "NetMerge.exe.sha256"):
			shaURL = a.URL
		}
	}
	if sha == "" && shaURL != "" {
		c2, cc := context.WithTimeout(context.Background(), 20*time.Second)
		if r2, err := httpGet(c2, shaURL); err == nil {
			b, _ := io.ReadAll(io.LimitReader(r2.Body, 4096))
			r2.Body.Close()
			if f := strings.Fields(string(b)); len(f) > 0 {
				sha = strings.ToLower(f[0])
			}
		}
		cc()
	}
	newer := newerVersion(rel.Tag, appVersion) && !rel.Pre
	updMu.Lock()
	upd.Status, upd.Error, upd.Checked = "idle", "", time.Now().Unix()
	upd.Latest, upd.Notes, upd.Page = strings.TrimPrefix(rel.Tag, "v"), rel.Body, rel.Page
	upd.asset, upd.sha = asset, sha
	upd.Available = newer && asset != ""
	notify := upd.Available && notifd != upd.Latest
	if notify {
		notifd = upd.Latest
	}
	ver := upd.Latest
	updMu.Unlock()
	if notify {
		logf("update available: %s (this is %s)", ver, appVersion)
		note("info", "NetMerge %s is available", ver)
		trayBalloon("NetMerge update", "Version "+ver+" is available. Open NetMerge to install it.")
	}
}

func okDownloadHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".github.com") || strings.HasSuffix(h, ".githubusercontent.com")
}

// installUpdate downloads the new NetMerge.exe, verifies it and hands over to the updater script.
func installUpdate() error {
	updMu.Lock()
	if !upd.Available || upd.Status != "idle" {
		updMu.Unlock()
		return fmt.Errorf("there is no update to install")
	}
	asset, sha := upd.asset, upd.sha
	upd.Status, upd.Progress, upd.Error = "downloading", 0, ""
	updMu.Unlock()
	fail := func(err error) error {
		updMu.Lock()
		upd.Status, upd.Error = "idle", err.Error()
		updMu.Unlock()
		logf("update failed: %v", err)
		return err
	}
	if !okDownloadHost(asset) && os.Getenv("NETMERGE_UPDATE_API") == "" {
		return fail(fmt.Errorf("the download address is not a GitHub address"))
	}
	if len(sha) != 64 {
		return fail(fmt.Errorf("the release has no fingerprint to check the download against"))
	}
	dir := filepath.Join(rootDir, "data", "update")
	_ = os.MkdirAll(dir, 0o755)
	dst := filepath.Join(dir, "NetMerge-new.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	resp, err := httpGet(ctx, asset)
	if err != nil {
		return fail(fmt.Errorf("download failed: %v", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fail(fmt.Errorf("download failed (HTTP %d)", resp.StatusCode))
	}
	f, err := os.Create(dst)
	if err != nil {
		return fail(err)
	}
	h := sha256.New()
	var got int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return fail(werr)
			}
			h.Write(buf[:n])
			got += int64(n)
			if resp.ContentLength > 0 {
				updMu.Lock()
				upd.Progress = int(got * 100 / resp.ContentLength)
				updMu.Unlock()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fail(fmt.Errorf("download interrupted: %v", rerr))
		}
	}
	f.Close()
	if hex.EncodeToString(h.Sum(nil)) != sha {
		_ = os.Remove(dst)
		return fail(fmt.Errorf("the download does not match its fingerprint, so it was discarded"))
	}
	if b, err := os.ReadFile(dst); err != nil || len(b) < 1<<20 || b[0] != 'M' || b[1] != 'Z' {
		_ = os.Remove(dst)
		return fail(fmt.Errorf("the downloaded file is not a program"))
	}
	updMu.Lock()
	upd.Status, upd.Progress = "installing", 100
	updMu.Unlock()
	logf("update downloaded and verified (%d bytes); installing", got)
	exe, _ := os.Executable()
	if err := startUpdater(dst, exe, filepath.Join(rootDir, "NetMerge-previous.exe")); err != nil {
		return fail(err)
	}
	go func() { time.Sleep(800 * time.Millisecond); requestQuit() }()
	return nil
}

func updateLoop() {
	time.Sleep(90 * time.Second)
	for {
		if !getSettings().NoUpdateCheck {
			checkUpdate(false)
		}
		time.Sleep(6 * time.Hour)
	}
}

func registerUpdateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/update", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var in struct {
				Action string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			switch in.Action {
			case "engine":
				go safely("engine download", downloadEngine)
			case "check":
				checkUpdate(true)
			case "install":
				if err := installUpdate(); err != nil {
					writeErr(w, 500, err.Error())
					return
				}
			}
		}
		writeJSON(w, updateSnapshot())
	}))
}
