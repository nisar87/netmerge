//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// removeVPNTestLeftovers undoes what the Windows VPN test (3.23.0–3.23.5, since removed) put on this PC: the helper
// app "NetMerge Windows VPN (test)", its "NetMerge" VPN entry, its loopback exemption, its test certificate and its
// folder. Runs once, before merging starts, and only where that test was used (data/winvpn exists).
func removeVPNTestLeftovers() {
	dir := filepath.Join(rootDir, "data", "winvpn")
	if _, err := os.Stat(dir); err != nil {
		return
	}
	logf("removing what the Windows VPN test left on this PC")
	out, err := powershell(`$p = Get-AppxPackage -Name NetMerge.WindowsVpnTest
Get-VpnConnection -Name 'NetMerge' -ErrorAction SilentlyContinue | Where-Object { $_.ConnectionStatus -eq 'Connected' } | ForEach-Object { rasdial 'NetMerge' /disconnect | Out-Null }
Remove-VpnConnection -Name 'NetMerge' -Force -ErrorAction SilentlyContinue
if ($p) { CheckNetIsolation.exe LoopbackExempt -d "-n=$($p.PackageFamilyName)" | Out-Null; Remove-AppxPackage -Package $p.PackageFullName; 'helper removed' } else { 'no helper installed' }`)
	logf("Windows VPN test clean-up: %s %v", strings.TrimSpace(string(out)), err)
	if out, err := hiddenCmd("certutil.exe", "-delstore", "TrustedPeople", "NetMerge Test").CombinedOutput(); err != nil {
		logf("Windows VPN test clean-up: certificate: %v %s", err, firstLine(string(out)))
	}
	_ = hiddenCmd("schtasks.exe", "/Delete", "/F", "/TN", "NetMerge VPN helper link").Run()
	if err := os.RemoveAll(dir); err != nil {
		logf("Windows VPN test clean-up: folder: %v", err)
	}
}
