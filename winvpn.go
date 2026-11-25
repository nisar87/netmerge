package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Windows VPN capture (test). With Settings > "How NetMerge catches traffic" set to the Windows VPN, the engine
// takes packets from NetMerge's VPN plug-in instead of its own Wintun adapter. The plug-in is a small Windows app
// package ("NetMerge Windows VPN (test)", built from winvpn/), downloaded from the winvpn-test pre-release,
// installed with a test certificate, and switched on and off through its netmerge-vpn: link. This is how a
// Microsoft Store edition would have to catch traffic; the test shows whether it is as fast and steady.

const winvpnPackage = "NetMerge.WindowsVpnTest"

var (
	wvMu        sync.Mutex // one sync at a time
	wvStateMu   sync.Mutex
	wvStatus    string // shown in the window
	wvConnected bool
	wvCfg       string // the plug-in settings it was connected with
	wvAgain     = make(chan struct{}, 1)
)

// usableVPNPort returns port when the engine can listen on it, otherwise a port Windows hands out itself. A port
// picked at random can fall in a range Windows keeps for itself (Hyper-V, WSL and Docker reserve ranges; binding
// there fails with "access forbidden"), so the free port comes from Windows, which never hands out a kept one.
func usableVPNPort(port int) int {
	if port > 0 {
		if c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}); err == nil {
			c.Close()
			return port
		}
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		return port
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

func setWinvpnStatus(s string) {
	wvStateMu.Lock()
	changed := wvStatus != s
	wvStatus = s
	wvStateMu.Unlock()
	if changed && s != "" {
		logf("Windows VPN (test): %s", s)
	}
}

func winvpnStatus() string {
	wvStateMu.Lock()
	defer wvStateMu.Unlock()
	return wvStatus
}

// winvpnPluginConfig is what the plug-in reads from the VPN profile: where the engine listens, the token, and the
// DNS server to give Windows (the engine answers DNS itself when the built-in DNS is on).
func winvpnPluginConfig(s Settings, dns bool) string {
	cfg := fmt.Sprintf("<nm><port>%d</port><token>%s</token><mtu>1500</mtu>", s.VPNPort, s.VPNToken)
	if dns {
		cfg += "<dns>198.18.0.2</dns>"
	}
	return cfg + "</nm>"
}

func winvpnLink(action, cfg string) string {
	if action == "connect" {
		return "netmerge-vpn:connect?cfg=" + base64.URLEncoding.EncodeToString([]byte(cfg))
	}
	return "netmerge-vpn:" + action
}

// winvpnSync brings the Windows VPN in line with the settings: connected while merging runs with the test switched
// on, disconnected otherwise. Called after every start, stop and settings change; calls that arrive while one runs
// make it run once more.
func winvpnSync() {
	if !wvMu.TryLock() {
		select {
		case wvAgain <- struct{}{}:
		default:
		}
		return
	}
	defer func() {
		wvMu.Unlock()
		select {
		case <-wvAgain:
			go winvpnSync()
		default:
		}
	}()
	s := getSettings()
	running, _, _, _ := engine.Status()
	want := s.Capture == "winvpn" && running
	wvStateMu.Lock()
	connected, lastCfg := wvConnected, wvCfg
	wvStateMu.Unlock()
	cfg := winvpnPluginConfig(s, dnsOn(s))
	switch {
	case want && (!connected || cfg != lastCfg):
		if err := winvpnEnsureHelper(); err != nil {
			setWinvpnStatus("helper not installed: " + err.Error())
			note("bad", "Windows VPN (test): the helper could not be installed: %v", err)
			return
		}
		setWinvpnStatus("connecting")
		res, err := winvpnRun("connect", cfg)
		if err != nil || res != "connected" {
			msg := res
			if err != nil {
				msg = err.Error()
			}
			setWinvpnStatus("not connected: " + msg)
			note("bad", "Windows VPN (test) did not connect: %s", msg)
			return
		}
		wvStateMu.Lock()
		wvConnected, wvCfg = true, cfg
		wvStateMu.Unlock()
		setWinvpnStatus("connected")
		note("info", "Windows VPN (test) connected: traffic now comes through Windows' own VPN")
	case !want && connected:
		res, err := winvpnRun("disconnect", "")
		if err != nil {
			res = err.Error()
		}
		wvStateMu.Lock()
		wvConnected, wvCfg = false, ""
		wvStateMu.Unlock()
		setWinvpnStatus("")
		logf("Windows VPN (test): %s", res)
	case !want && s.Capture != "winvpn":
		setWinvpnStatus("")
	}
}

// dnsOn: does the engine answer DNS itself (the built-in DNS)? Then Windows is told to ask it through the VPN.
func dnsOn(s Settings) bool {
	return s.BuiltinDNS
}

// winvpnEnsureHelper installs (or updates) the helper package when the published one differs from what was
// installed last.
func winvpnEnsureHelper() error {
	if !winvpnSupported() {
		return fmt.Errorf("only on Windows")
	}
	dir := filepath.Join(rootDir, "data", "winvpn")
	_ = os.MkdirAll(dir, 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	base := "https://github.com/" + updateRepo + "/releases/download/winvpn-test/"
	if u := os.Getenv("NETMERGE_WINVPN_URL"); u != "" {
		base = u
	}
	want, err := fetchSmall(ctx, base+"NetMergeVpnTest.zip.sha256")
	if err != nil {
		if pfn := winvpnInstalledPFN(); pfn != "" {
			logf("Windows VPN (test): cannot check for a newer helper (%v); using the installed one", err)
			return nil
		}
		return fmt.Errorf("download the helper's fingerprint: %v", err)
	}
	want = strings.ToLower(strings.Fields(want + " x")[0])
	if len(want) != 64 {
		return fmt.Errorf("the helper's fingerprint is unreadable")
	}
	had, _ := os.ReadFile(filepath.Join(dir, "installed.sha256"))
	if strings.TrimSpace(string(had)) == want && winvpnInstalledPFN() != "" {
		return nil
	}
	setWinvpnStatus("downloading the helper")
	zipPath := filepath.Join(dir, "NetMergeVpnTest.zip")
	if err := fetchFile(ctx, base+"NetMergeVpnTest.zip", zipPath); err != nil {
		return fmt.Errorf("download the helper: %v", err)
	}
	if got, err := fileSHA256(zipPath); err != nil || got != want {
		return fmt.Errorf("the downloaded helper does not match its fingerprint")
	}
	unpacked := filepath.Join(dir, "pkg")
	_ = os.RemoveAll(unpacked)
	if err := unzipTo(zipPath, unpacked); err != nil {
		return fmt.Errorf("unpack the helper: %v", err)
	}
	setWinvpnStatus("installing the helper")
	if err := winvpnInstall(unpacked); err != nil {
		return err
	}
	_ = os.WriteFile(filepath.Join(dir, "installed.sha256"), []byte(want), 0o644)
	return nil
}

func fetchSmall(ctx context.Context, u string) (string, error) {
	resp, err := httpGet(ctx, u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return string(b), err
}

func fetchFile(ctx context.Context, u, dst string) error {
	resp, err := httpGet(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 200<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// unzipTo unpacks a zip into dir, refusing names that would land outside it.
func unzipTo(zipPath, dir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	root, _ := filepath.Abs(dir)
	for _, f := range r.File {
		name := filepath.FromSlash(strings.ReplaceAll(f.Name, `\`, "/"))
		dst := filepath.Join(root, name)
		if !strings.HasPrefix(dst, root+string(os.PathSeparator)) {
			return fmt.Errorf("bad name in the zip: %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(dst, 0o755)
			continue
		}
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(dst)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(in, 200<<20))
		in.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// winvpnDisconnectNow disconnects the Windows VPN before NetMerge quits (waiting for the answer), so Windows does not
// keep sending the PC's traffic to an engine that is gone.
func winvpnDisconnectNow() {
	wvStateMu.Lock()
	connected := wvConnected
	wvConnected, wvCfg = false, ""
	wvStateMu.Unlock()
	if !connected {
		return
	}
	res, err := winvpnRun("disconnect", "")
	logf("Windows VPN (test) on quitting: %s %v", res, err)
}

// winvpnStartupCleanup: a NetMerge that closed unexpectedly may have left the "NetMerge" VPN connected; with the test
// switched off, disconnect it once at start (only when the helper is installed at all).
func winvpnStartupCleanup() {
	if getSettings().Capture == "winvpn" || !winvpnSupported() || winvpnInstalledPFN() == "" {
		return
	}
	res, err := winvpnRun("disconnect", "")
	logf("Windows VPN (test) left from before: %s %v", res, err)
}
