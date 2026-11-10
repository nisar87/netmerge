package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The network engine (mihomo) is a separate open-source program. A fresh install does not contain it, so
// NetMerge downloads the pinned version from the official project's GitHub release and checks its SHA-256.

const engineVersion = "1.19.33"

// engineZipSHA256 is the fingerprint of mihomo-windows-amd64-v1.19.33.zip as published by the mihomo project. It is
// pinned here, so a release changed after the fact (or a different file served under the same name) is refused
// even if its own listed fingerprint matches. Change it together with engineVersion.
var engineZipSHA256 = "6ab2180875f9f6fff8b19386d760daec257f66b45f72b3afc650634272fa837a"

var (
	edMu     sync.Mutex
	edStatus = "" // "" | "downloading" | "error"
	edPct    int
	edErr    string
)

func engineDownloadState() (string, int, string) {
	edMu.Lock()
	defer edMu.Unlock()
	return edStatus, edPct, edErr
}

func engineMissing() bool { _, err := os.Stat(mihomoExe()); return err != nil }

// downloadEngine fetches mihomo into the core folder. Safe to call more than once.
func downloadEngine() {
	edMu.Lock()
	if edStatus == "downloading" {
		edMu.Unlock()
		return
	}
	edStatus, edPct, edErr = "downloading", 0, ""
	edMu.Unlock()
	err := func() error {
		tag := "v" + engineVersion
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		resp, err := httpGet(ctx, updateAPI+"/repos/MetaCubeX/mihomo/releases/tags/"+tag)
		if err != nil {
			return fmt.Errorf("could not reach GitHub")
		}
		var rel ghRelease
		err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rel)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			return fmt.Errorf("could not read the engine release (HTTP %d)", resp.StatusCode)
		}
		want := "mihomo-windows-amd64-" + tag + ".zip"
		var asset, sha string
		for _, a := range rel.Assets {
			if a.Name == want {
				asset, sha = a.URL, strings.TrimPrefix(strings.ToLower(a.Digest), "sha256:")
			}
		}
		if asset == "" {
			return fmt.Errorf("the engine file %s was not found in the release", want)
		}
		if engineZipSHA256 != "" {
			if sha != "" && sha != engineZipSHA256 {
				return fmt.Errorf("the engine release lists a different fingerprint than the one NetMerge expects, so it was not used")
			}
			sha = engineZipSHA256
		}
		if len(sha) != 64 {
			return fmt.Errorf("the engine release has no fingerprint to check against")
		}
		r2, err := httpGet(ctx, asset)
		if err != nil {
			return fmt.Errorf("engine download failed")
		}
		defer r2.Body.Close()
		if r2.StatusCode != 200 {
			return fmt.Errorf("engine download failed (HTTP %d)", r2.StatusCode)
		}
		var buf bytes.Buffer
		h := sha256.New()
		b := make([]byte, 64<<10)
		for {
			n, rerr := r2.Body.Read(b)
			if n > 0 {
				buf.Write(b[:n])
				h.Write(b[:n])
				if r2.ContentLength > 0 {
					edMu.Lock()
					edPct = int(int64(buf.Len()) * 100 / r2.ContentLength)
					edMu.Unlock()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return fmt.Errorf("engine download interrupted")
			}
		}
		if hex.EncodeToString(h.Sum(nil)) != sha {
			return fmt.Errorf("the engine download does not match its fingerprint")
		}
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".exe") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(rc, 200<<20))
			rc.Close()
			if err != nil || len(data) < 1<<20 {
				return fmt.Errorf("the engine file inside the download is damaged")
			}
			if err := os.MkdirAll(filepath.Dir(mihomoExe()), 0o755); err != nil {
				return fmt.Errorf("cannot create the folder %s for the engine: %v", filepath.Dir(mihomoExe()), err)
			}
			return atomicWrite(mihomoExe(), data)
		}
		return fmt.Errorf("no program found inside the engine download")
	}()
	edMu.Lock()
	if err != nil {
		edStatus, edErr = "error", err.Error()
		logf("engine download: %v", err)
	} else {
		edStatus, edErr = "", ""
		logf("engine %s downloaded", engineVersion)
	}
	edMu.Unlock()
}
