//go:build !windows

package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func processPath(pid int) string {
	p, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/exe")
	if err != nil {
		return ""
	}
	if st, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if i := strings.LastIndexByte(string(st), ')'); i > 0 && len(st) > i+2 && st[i+2] == 'Z' {
			return "" // exited, waiting to be reaped
		}
	}
	return strings.TrimSuffix(p, " (deleted)")
}

func processesRunning(path string) []int {
	var out []int
	for _, pid := range allPids() {
		if samePath(processPath(pid), path) {
			out = append(out, pid)
		}
	}
	return out
}

func allPids() []int {
	ents, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range ents {
		if pid, err := strconv.Atoi(e.Name()); err == nil {
			out = append(out, pid)
		}
	}
	return out
}

func killPid(pid int) { _ = syscall.Kill(pid, syscall.SIGKILL) }
