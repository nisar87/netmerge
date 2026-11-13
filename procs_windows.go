//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var procQueryFullProcessImageNameW = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryFullProcessImageNameW")

// processPath returns the full program path of a running process, or "" if it is not running.
func processPath(pid int) string {
	h, err := syscall.OpenProcess(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if syscall.GetExitCodeProcess(h, &code) == nil && code != 259 /* STILL_ACTIVE */ {
		return ""
	}
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// processesRunning lists the processes running the program at path.
func processesRunning(path string) []int {
	var out []int
	for _, pid := range allPids() {
		if samePath(processPath(pid), path) {
			out = append(out, pid)
		}
	}
	return out
}

// allPids lists every running process.
func allPids() []int {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []int
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		if pe.ProcessID != 0 {
			out = append(out, int(pe.ProcessID))
		}
	}
	return out
}

func killPid(pid int) {
	h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	_ = syscall.TerminateProcess(h, 1)
	syscall.CloseHandle(h)
}
