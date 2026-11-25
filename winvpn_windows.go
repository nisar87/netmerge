//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func winvpnSupported() bool { return true }

// winvpnInstalledPFN returns the helper's package family name, or "" when it is not installed.
func winvpnInstalledPFN() string {
	out, err := powershell("(Get-AppxPackage -Name " + winvpnPackage + ").PackageFamilyName")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// winvpnInstall trusts the helper's test certificate (on this PC only, for apps), installs the package with the
// Windows parts it needs, and lets it reach the engine on 127.0.0.1 (Windows keeps app packages off the PC's own
// addresses unless told otherwise).
func winvpnInstall(dir string) error {
	cer := filepath.Join(dir, "NetMergeVpnTest.cer")
	msix := filepath.Join(dir, "NetMergeVpn.msix")
	if _, err := os.Stat(msix); err != nil {
		return fmt.Errorf("the helper package is missing from the download")
	}
	if out, err := hiddenCmd("certutil.exe", "-addstore", "TrustedPeople", cer).CombinedOutput(); err != nil {
		return fmt.Errorf("trust the helper's certificate: %v: %s", err, firstLineOf(string(out)))
	}
	deps := ""
	if m, _ := filepath.Glob(filepath.Join(dir, "deps", "*")); len(m) > 0 {
		q := make([]string, len(m))
		for i, p := range m {
			q[i] = "'" + strings.ReplaceAll(p, "'", "''") + "'"
		}
		deps = " -DependencyPath @(" + strings.Join(q, ",") + ")"
	}
	script := "$ErrorActionPreference='Stop'; Add-AppxPackage -Path '" + strings.ReplaceAll(msix, "'", "''") + "'" + deps +
		" -ForceUpdateFromAnyVersion -ForceApplicationShutdown"
	if out, err := powershell(script); err != nil {
		return fmt.Errorf("install the helper: %v: %s", err, firstLineOf(string(out)))
	}
	pfn := winvpnInstalledPFN()
	if pfn == "" {
		return fmt.Errorf("the helper was installed but Windows does not list it")
	}
	if out, err := hiddenCmd("CheckNetIsolation.exe", "LoopbackExempt", "-a", "-n="+pfn).CombinedOutput(); err != nil {
		logf("Windows VPN (test): loopback exemption: %v: %s", err, firstLineOf(string(out)))
	}
	logf("Windows VPN (test): helper installed (%s)", pfn)
	return nil
}

// winvpnRun asks the helper to connect or disconnect the "NetMerge" VPN and waits for its answer. The link is opened
// through Explorer so the helper starts as the signed-in user, not with NetMerge's administrator rights (Windows
// does not start app packages from an elevated program).
func winvpnRun(action, cfg string) (string, error) {
	pfn := winvpnInstalledPFN()
	if pfn == "" {
		return "", fmt.Errorf("the helper is not installed")
	}
	status := filepath.Join(os.Getenv("LOCALAPPDATA"), "Packages", pfn, "LocalState", "status.txt")
	began := time.Now()
	if err := winvpnOpenLink(winvpnLink(action, cfg)); err != nil {
		return "", err
	}
	for time.Since(began) < 45*time.Second {
		time.Sleep(300 * time.Millisecond)
		st, err := os.Stat(status)
		if err != nil || st.ModTime().Before(began.Add(-time.Second)) {
			continue
		}
		b, err := os.ReadFile(status)
		if err != nil {
			continue
		}
		// "<time> <result>"
		res := strings.TrimSpace(string(b))
		if i := strings.IndexByte(res, ' '); i > 0 {
			res = res[i+1:]
		}
		return res, nil
	}
	return "", fmt.Errorf("the helper did not answer within 45 seconds")
}

// winvpnOpenLink opens the helper's link as the signed-in user without administrator rights. Windows does not start
// app packages for an elevated program, and Explorer started from one does not hand the link on either (seen on a
// real PC: nothing happened). A one-off scheduled task with "limited" rights runs in the user's own session, like
// typing the link into Run, and is removed right after.
// Windows limits a task command to 261 characters; the link with its settings is far shorter.
func winvpnOpenLink(link string) error {
	const task = "NetMerge VPN helper link"
	out, err := hiddenCmd("schtasks.exe", "/Create", "/F", "/TN", task, "/SC", "ONCE", "/ST", "00:00",
		"/RL", "LIMITED", "/IT", "/TR", winvpnTaskCommand(link)).CombinedOutput()
	if err == nil {
		out, err = hiddenCmd("schtasks.exe", "/Run", "/TN", task).CombinedOutput()
		go func() {
			time.Sleep(10 * time.Second)
			_ = hiddenCmd("schtasks.exe", "/Delete", "/F", "/TN", task).Run()
		}()
		if err == nil {
			return nil
		}
	}
	logf("Windows VPN (test): starting the helper as the signed-in user did not work (%v: %s)", err, firstLineOf(string(out)))
	return fmt.Errorf("could not start the helper as the signed-in user")
}

// winvpnTaskCommand is what the task runs: PowerShell's Start-Process opens the link the way the Run box does.
// (Explorer was tried first: given this link it opened a "This PC" window instead, seen on a real PC.)
func winvpnTaskCommand(link string) string {
	return "powershell.exe -NoProfile -WindowStyle Hidden -Command Start-Process '" + strings.ReplaceAll(link, "'", "''") + "'"
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
