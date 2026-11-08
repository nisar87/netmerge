//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const createNoWindow = 0x08000000

var (
	iphlpapi    = syscall.NewLazyDLL("iphlpapi.dll")
	pGetIfTable = iphlpapi.NewProc("GetIfTable")
	shell32     = syscall.NewLazyDLL("shell32.dll")
	pShellExec  = shell32.NewProc("ShellExecuteW")
	user32      = syscall.NewLazyDLL("user32.dll")
	pMessageBox = user32.NewProc("MessageBoxW")
)

func hiddenCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return c
}

func powershell(script string) ([]byte, error) {
	c := hiddenCmd("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	return c.Output()
}

func listAdapters() ([]Adapter, error) {
	script := `$ErrorActionPreference='SilentlyContinue'
$r = @(Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null -and $_.NetAdapter.Status -eq 'Up' } | ForEach-Object {
 [pscustomobject]@{ name=$_.InterfaceAlias; index=[int]$_.InterfaceIndex; desc=[string]$_.InterfaceDescription;
  ip=[string](@($_.IPv4Address)[0].IPAddress); gateway=[string](@($_.IPv4DefaultGateway)[0].NextHop);
  linkRate=[string]$_.NetAdapter.LinkSpeed; up=$true } })
ConvertTo-Json -InputObject $r -Compress`
	out, err := powershell(script)
	if err != nil {
		return nil, err
	}
	out = []byte(strings.TrimSpace(string(out)))
	var list []Adapter
	if len(out) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse adapters: %v: %s", err, string(out))
	}
	return list, nil
}

// ifCounters returns total received/sent bytes per interface index (32-bit counters, wrap handled by caller).
func ifCounters() map[int][2]uint32 {
	res := map[int][2]uint32{}
	var size uint32
	pGetIfTable.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 {
		return res
	}
	buf := make([]byte, size+1024)
	size = uint32(len(buf))
	r, _, _ := pGetIfTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r != 0 {
		return res
	}
	n := *(*uint32)(unsafe.Pointer(&buf[0]))
	const rowSize = 860
	for i := 0; i < int(n); i++ {
		off := 4 + i*rowSize
		if off+rowSize > len(buf) {
			break
		}
		row := buf[off : off+rowSize]
		idx := *(*uint32)(unsafe.Pointer(&row[512]))
		in := *(*uint32)(unsafe.Pointer(&row[552]))
		outb := *(*uint32)(unsafe.Pointer(&row[576]))
		res[int(idx)] = [2]uint32{in, outb}
	}
	return res
}

// bindControl forces a socket out of a specific interface (bypasses the merge tunnel), like mihomo does.
func bindControl(ifIndex int) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			// IP_UNICAST_IF = 31, value must be in network byte order
			v := int(uint32(ifIndex)>>24 | (uint32(ifIndex)>>8)&0xff00 | (uint32(ifIndex)<<8)&0xff0000 | uint32(ifIndex)<<24)
			serr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, 31, v)
		})
		if err != nil {
			return err
		}
		return serr
	}
}

func isAdmin() bool {
	var tok syscall.Token
	p, _ := syscall.GetCurrentProcess()
	if err := syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &tok); err != nil {
		return false
	}
	defer tok.Close()
	var elevated uint32
	var ret uint32
	err := syscall.GetTokenInformation(tok, 20 /*TokenElevation*/, (*byte)(unsafe.Pointer(&elevated)), 4, &ret)
	return err == nil && elevated != 0
}

func relaunchAsAdmin() bool {
	exe, _ := os.Executable()
	args := strings.Join(os.Args[1:], " ")
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	par, _ := syscall.UTF16PtrFromString(args)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	r, _, _ := pShellExec.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(par)), uintptr(unsafe.Pointer(dir)), 1)
	return r > 32
}

func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	pMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}

var (
	pEnumWindows    = user32.NewProc("EnumWindows")
	pGetClassName   = user32.NewProc("GetClassNameW")
	pGetWindowText  = user32.NewProc("GetWindowTextW")
	pIsWindowVis    = user32.NewProc("IsWindowVisible")
	pIsIconic       = user32.NewProc("IsIconic")
	pShowWindow     = user32.NewProc("ShowWindow")
	pBringToTop     = user32.NewProc("BringWindowToTop")
	pSetForeground  = user32.NewProc("SetForegroundWindow")
	enumFocusCb     = syscall.NewCallback(enumFocus)
	focusedExisting bool
)

// enumFocus looks for the NetMerge app window (an Edge/Chrome "app" window titled NetMerge) and brings it up.
func enumFocus(hwnd, _ uintptr) uintptr {
	if v, _, _ := pIsWindowVis.Call(hwnd); v == 0 {
		return 1
	}
	cls := make([]uint16, 64)
	pGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&cls[0])), 64)
	if syscall.UTF16ToString(cls) != "Chrome_WidgetWin_1" {
		return 1
	}
	title := make([]uint16, 128)
	pGetWindowText.Call(hwnd, uintptr(unsafe.Pointer(&title[0])), 128)
	if !strings.HasPrefix(syscall.UTF16ToString(title), "NetMerge") {
		return 1
	}
	if m, _, _ := pIsIconic.Call(hwnd); m != 0 {
		pShowWindow.Call(hwnd, 9) // SW_RESTORE
	}
	pBringToTop.Call(hwnd)
	pSetForeground.Call(hwnd)
	focusedExisting = true
	return 0 // stop
}

// focusExistingWindow brings the already open NetMerge window to the front; false if there is none.
func focusExistingWindow() bool {
	focusedExisting = false
	pEnumWindows.Call(enumFocusCb, 0)
	return focusedExisting
}

func openWindow(url string) {
	if focusExistingWindow() {
		return
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("LocalAppData"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
	}
	profile := filepath.Join(rootDir, "data", "ui-profile")
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			cmd := exec.Command(c, "--app="+url, "--user-data-dir="+profile, "--window-size=1280,860",
				"--no-first-run", "--no-default-browser-check", "--disable-extensions")
			if err := cmd.Start(); err == nil {
				go cmd.Wait()
				return
			}
		}
	}
	_ = hiddenCmd("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func killProcessByName(name string) {
	_ = hiddenCmd("taskkill", "/F", "/IM", name).Run()
}

func setAutoStart(on bool) error {
	exe, _ := os.Executable()
	if on {
		out, err := hiddenCmd("schtasks", "/Create", "/TN", "NetMerge", "/TR", `"`+exe+`" --background`,
			"/SC", "ONLOGON", "/RL", "HIGHEST", "/F").CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		// schtasks' defaults would stop NetMerge after 72 hours and never start it on battery power
		if out, err := hiddenCmd("powershell", "-NoProfile", "-Command",
			"$s=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -StartWhenAvailable;"+
				"Set-ScheduledTask -TaskName NetMerge -Settings $s | Out-Null").CombinedOutput(); err != nil {
			logf("autostart: could not relax the task limits: %v %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	_ = hiddenCmd("schtasks", "/Delete", "/TN", "NetMerge", "/F").Run()
	return nil
}

func createShortcut() {
	exe, _ := os.Executable()
	script := fmt.Sprintf(`$s=(New-Object -ComObject WScript.Shell).CreateShortcut([Environment]::GetFolderPath('Desktop')+'\NetMerge.lnk');$s.TargetPath='%s';$s.WorkingDirectory='%s';$s.IconLocation='%s,0';$s.Save()`,
		exe, filepath.Dir(exe), exe)
	_, _ = powershell(script)
}

func startEngineProcess(exe string, args ...string) (*exec.Cmd, error) {
	c := hiddenCmd(exe, args...)
	return c, c.Start()
}

// setShareFirewall opens (or closes) the proxy-sharing ports to the local network only.
func setShareFirewall(on bool, port, ssPort int) {
	const name = "name=NetMerge proxy sharing"
	_ = hiddenCmd("netsh", "advfirewall", "firewall", "delete", "rule", name).Run()
	if !on {
		return
	}
	for _, proto := range []string{"TCP", "UDP"} {
		ports := fmt.Sprintf("%d,%d", port, ssPort)
		if proto == "UDP" {
			ports = fmt.Sprintf("%d", ssPort)
		}
		out, err := hiddenCmd("netsh", "advfirewall", "firewall", "add", "rule", name, "dir=in", "action=allow",
			"protocol="+proto, "localport="+ports, "remoteip=localsubnet", "profile=any").CombinedOutput()
		if err != nil {
			logf("firewall rule for proxy sharing failed: %v %s", err, strings.TrimSpace(string(out)))
		}
	}
}

var (
	pnMu   sync.Mutex
	pnAt   time.Time
	pnList []string
)

// publicNetworks lists the connections Windows treats as "Public" (cafés, hotels); cached for a minute.
func publicNetworks() []string {
	pnMu.Lock()
	defer pnMu.Unlock()
	if time.Since(pnAt) < time.Minute {
		return pnList
	}
	pnAt = time.Now()
	pnList = nil
	out, err := powershell(`Get-NetConnectionProfile | Where-Object { $_.NetworkCategory -eq 'Public' } | ForEach-Object { $_.InterfaceAlias }`)
	if err == nil {
		for _, ln := range strings.Split(string(out), "\n") {
			if ln = strings.TrimSpace(ln); ln != "" && !strings.HasPrefix(ln, "Meta") {
				pnList = append(pnList, ln)
			}
		}
	}
	return pnList
}

// startUpdater runs a small script that waits for NetMerge to exit, swaps in the new program (keeping the old
// one as the "previous" copy), and starts it again. If the swap fails it puts the old one back.
func startUpdater(newExe, curExe, prevExe string) error {
	script := filepath.Join(filepath.Dir(newExe), "apply-update.ps1")
	body := `param([int]$ProcId,[string]$New,[string]$Cur,[string]$Prev)
$ErrorActionPreference = 'Stop'
try { Wait-Process -Id $ProcId -Timeout 40 } catch {}
Get-Process mihomo -ErrorAction SilentlyContinue | Stop-Process -Force
Start-Sleep 4
try {
  Copy-Item -Force $Cur $Prev
  Move-Item -Force $New $Cur
} catch {
  if (Test-Path $Prev) { Copy-Item -Force $Prev $Cur }
}
Start-Process $Cur
`
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		return err
	}
	c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", script,
		"-ProcId", fmt.Sprint(os.Getpid()), "-New", newExe, "-Cur", curExe, "-Prev", prevExe)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow | 0x00000200}
	return c.Start()
}

// openPath opens a file with its normal program, or shows it selected in Explorer. The command line is written by hand
// because Explorer and rundll32 want the path in quotes exactly like this, spaces and all.
func openPath(path string, reveal bool) error {
	var c *exec.Cmd
	if reveal {
		c = hiddenCmd("explorer.exe")
		c.SysProcAttr.CmdLine = `explorer.exe /select,"` + path + `"`
	} else {
		c = hiddenCmd("rundll32.exe")
		c.SysProcAttr.CmdLine = `rundll32.exe url.dll,FileProtocolHandler "` + path + `"`
	}
	return c.Start()
}

// dlPower sleeps or shuts the PC down after the downloads (a shutdown has a one minute warning that can be cancelled).
func dlPower(kind string) {
	switch kind {
	case "shutdown":
		_ = hiddenCmd("shutdown.exe", "/s", "/t", "60", "/c", "NetMerge: downloads finished").Start()
	case "sleep":
		_ = hiddenCmd("rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0").Start()
	}
}

func dlPowerCancel() error { return hiddenCmd("shutdown.exe", "/a").Run() }

// freeSpace returns the bytes a normal program can still write to the drive that holds dir.
func freeSpace(dir string) (int64, error) {
	p, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var avail, total, free uint64
	r, _, e := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW").Call(uintptr(unsafe.Pointer(p)),
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
	if r == 0 {
		return 0, e
	}
	return int64(avail), nil
}

// markFromInternet writes the "this file came from the internet" mark that browsers write, so Windows SmartScreen
// and Office protected view treat the file the way they treat any other download. Ignored where it is not supported.
func markFromInternet(path, link string) {
	ref := ""
	if link != "" && len(link) < 2000 && !strings.ContainsAny(link, "\r\n") {
		if i := strings.IndexByte(link, '?'); i >= 0 {
			link = link[:i]
		}
		ref = "HostUrl=" + link + "\r\n"
	}
	_ = os.WriteFile(path+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"+ref), 0o644)
}
