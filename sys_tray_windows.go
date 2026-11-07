//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Notification-area icon: green while merging, grey when stopped; tooltip shows the live speed;
// click = open the window; right-click = open / start-stop merging / quit.

var (
	pRegisterClassEx  = user32.NewProc("RegisterClassExW")
	pCreateWindowEx   = user32.NewProc("CreateWindowExW")
	pDefWindowProc    = user32.NewProc("DefWindowProcW")
	pGetMessage       = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessage  = user32.NewProc("DispatchMessageW")
	pPostMessage      = user32.NewProc("PostMessageW")
	pSetTimer         = user32.NewProc("SetTimer")
	pCreatePopupMenu  = user32.NewProc("CreatePopupMenu")
	pAppendMenu       = user32.NewProc("AppendMenuW")
	pTrackPopupMenu   = user32.NewProc("TrackPopupMenu")
	pDestroyMenu      = user32.NewProc("DestroyMenu")
	pSetForegroundWin = user32.NewProc("SetForegroundWindow")
	pGetCursorPos     = user32.NewProc("GetCursorPos")
	pCreateIcon       = user32.NewProc("CreateIcon")
	pRegisterWinMsg   = user32.NewProc("RegisterWindowMessageW")
	pShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	kernel32t         = syscall.NewLazyDLL("kernel32.dll")
	pGetModuleHandle  = kernel32t.NewProc("GetModuleHandleW")
)

const (
	wmUser     = 0x0400
	wmTray     = wmUser + 1
	wmTimer    = 0x0113
	wmClose    = 0x0010
	wmDestroy  = 0x0002
	wmLBtnUp   = 0x0202
	wmRBtnUp   = 0x0205
	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 1
	nifIcon    = 2
	nifTip     = 4
	nifInfo    = 0x10
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        [16]byte
	BalloonIcon     uintptr
}

type winMsg struct {
	Wnd     uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      [2]int32
}

var (
	trayWnd         atomic.Uintptr
	trayURL         string
	trayOn, trayOff uintptr
	trayTaskbarMsg  uint32
	trayRunning     bool
)

func makeIcon(r, g, b byte) uintptr {
	const n = 16
	xor := make([]byte, n*n*4)
	and := make([]byte, n*n/8)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-7.5, float64(y)-7.5
			i := (y*n + x) * 4
			if dx*dx+dy*dy <= 7.2*7.2 {
				xor[i], xor[i+1], xor[i+2], xor[i+3] = b, g, r, 255
			}
		}
	}
	h, _, _ := pCreateIcon.Call(0, n, n, 1, 32, uintptr(unsafe.Pointer(&and[0])), uintptr(unsafe.Pointer(&xor[0])))
	return h
}

func trayTip() (string, bool) {
	sn := getSnapshot()
	if !sn.Running {
		return "NetMerge - merging is off", false
	}
	return fmt.Sprintf("NetMerge  down %.0f Mbps  up %.0f Mbps", sn.TotalDown*8/1e6, sn.TotalUp*8/1e6), true
}

func trayNotify(msg uintptr) {
	hwnd := trayWnd.Load()
	if hwnd == 0 {
		return
	}
	tip, on := trayTip()
	nid := notifyIconData{Wnd: hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray}
	nid.Size = uint32(unsafe.Sizeof(nid))
	if on {
		nid.Icon = trayOn
	} else {
		nid.Icon = trayOff
	}
	t, _ := syscall.UTF16FromString(tip)
	copy(nid.Tip[:127], t)
	pShellNotifyIcon.Call(msg, uintptr(unsafe.Pointer(&nid)))
}

func trayMenu(hwnd uintptr) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	add := func(id uintptr, text string) {
		p, _ := syscall.UTF16PtrFromString(text)
		pAppendMenu.Call(menu, 0, id, uintptr(unsafe.Pointer(p)))
	}
	add(1, "Open NetMerge")
	if _, on := trayTip(); on {
		add(2, "Stop merging")
	} else {
		add(2, "Start merging")
	}
	add(3, "Quit NetMerge")
	var pt [2]int32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWin.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, 0x100|0x80, uintptr(pt[0]), uintptr(pt[1]), 0, hwnd, 0)
	pPostMessage.Call(hwnd, 0, 0, 0)
	switch cmd {
	case 1:
		go safely("tray open", func() { openWindow(trayURL) })
	case 2:
		go safely("tray power", func() {
			manualPower.Store(true)
			if _, on := trayTip(); on {
				engine.Stop()
			} else if err := engine.Start(); err != nil {
				logf("start from tray failed: %v", err)
			}
		})
	case 3:
		requestQuit()
	}
}

func trayProc(hwnd, msg, wp, lp uintptr) uintptr {
	defer func() { _ = recover() }()
	switch uint32(msg) {
	case wmTray:
		switch lp & 0xffff {
		case wmLBtnUp:
			go safely("tray open", func() { openWindow(trayURL) })
		case wmRBtnUp:
			trayMenu(hwnd)
		}
		return 0
	case wmTimer:
		trayNotify(nimModify)
		return 0
	case wmClose:
		trayNotify(nimDelete)
		trayWnd.Store(0)
		pPostMessage.Call(0, 0x0012, 0, 0) // WM_QUIT
		return 0
	}
	if trayTaskbarMsg != 0 && uint32(msg) == trayTaskbarMsg { // explorer restarted: put the icon back
		trayNotify(nimAdd)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wp, lp)
	return r
}

func runTray(url string) {
	runtime.LockOSThread()
	trayURL = url
	trayOn, trayOff = makeIcon(34, 197, 94), makeIcon(140, 140, 140)
	inst, _, _ := pGetModuleHandle.Call(0)
	cls, _ := syscall.UTF16PtrFromString("NetMergeTray")
	wc := wndClassEx{Instance: inst, WndProc: syscall.NewCallback(trayProc), ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, _ := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		logf("tray: could not register window class")
		return
	}
	title, _ := syscall.UTF16PtrFromString("NetMerge")
	hwnd, _, _ := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, inst, 0)
	if hwnd == 0 {
		logf("tray: could not create window")
		return
	}
	tb, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	m, _, _ := pRegisterWinMsg.Call(uintptr(unsafe.Pointer(tb)))
	trayTaskbarMsg = uint32(m)
	trayWnd.Store(hwnd)
	trayNotify(nimAdd)
	pSetTimer.Call(hwnd, 1, 2000, 0)
	var msg winMsg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// trayRemove takes the icon out of the notification area (called on quit).
func trayRemove() {
	if h := trayWnd.Load(); h != 0 {
		trayNotify(nimDelete)
		trayWnd.Store(0)
	}
}

// trayBalloon shows a short notification next to the tray icon.
func trayBalloon(title, text string) {
	hwnd := trayWnd.Load()
	if hwnd == 0 {
		return
	}
	nid := notifyIconData{Wnd: hwnd, ID: 1, Flags: nifInfo, InfoFlags: 1}
	nid.Size = uint32(unsafe.Sizeof(nid))
	t, _ := syscall.UTF16FromString(text)
	copy(nid.Info[:255], t)
	h, _ := syscall.UTF16FromString(title)
	copy(nid.InfoTitle[:63], h)
	pShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}
