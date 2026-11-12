package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

var (
	k32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessMemory = k32.NewProc("K32GetProcessMemoryInfo")
	iphlp                = syscall.NewLazyDLL("iphlpapi.dll")
	procGetIpForward     = iphlp.NewProc("GetIpForwardTable")
)

// procStats: working set now, peak working set, CPU time (user + kernel) of a process
func procStats(pid int) (uint64, uint64, time.Duration, error) {
	h, err := syscall.OpenProcess(0x1000|0x0010, false, uint32(pid)) // query limited information | VM read
	if err != nil {
		return 0, 0, 0, err
	}
	defer syscall.CloseHandle(h)
	var pmc [9]uintptr // PROCESS_MEMORY_COUNTERS: cb+faults, peakWS, WS, ...
	pmc[0] = unsafe.Sizeof(pmc)
	if r, _, e := procGetProcessMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc[0])), unsafe.Sizeof(pmc)); r == 0 {
		return 0, 0, 0, e
	}
	var c, x, k, u syscall.Filetime
	if err := syscall.GetProcessTimes(h, &c, &x, &k, &u); err != nil {
		return 0, 0, 0, err
	}
	cpu := time.Duration((int64(k.HighDateTime)<<32|int64(k.LowDateTime))+(int64(u.HighDateTime)<<32|int64(u.LowDateTime))) * 100
	return uint64(pmc[2]), uint64(pmc[1]), cpu, nil
}

// defaultInterface: the adapter of the IPv4 default route with the lowest metric
func defaultInterface() (string, error) {
	size := uint32(64 * 1024)
	buf := make([]byte, size)
	if r, _, _ := procGetIpForward.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 1); r != 0 {
		return "", fmt.Errorf("GetIpForwardTable: %d", r)
	}
	n := int(binary.LittleEndian.Uint32(buf))
	best, bestMetric := -1, uint32(1<<31)
	for i := 0; i < n && 4+(i+1)*56 <= len(buf); i++ {
		r := buf[4+i*56:]
		dst, mask := binary.LittleEndian.Uint32(r[0:4]), binary.LittleEndian.Uint32(r[4:8])
		idx, metric := binary.LittleEndian.Uint32(r[16:20]), binary.LittleEndian.Uint32(r[36:40])
		if dst == 0 && mask == 0 && metric < bestMetric {
			best, bestMetric = int(idx), metric
		}
	}
	if best < 0 {
		return "", fmt.Errorf("no default route")
	}
	ifi, err := net.InterfaceByIndex(best)
	if err != nil {
		return "", err
	}
	return ifi.Name, nil
}

func setupLocal() (func(), error) { return nil, fmt.Errorf("-local needs Linux") }
func interrupt(c *exec.Cmd)       {}
