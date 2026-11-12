//go:build windows

package main

import (
	"time"
	"unsafe"
)

var (
	pGetIfTable2    = iphlpapi.NewProc("GetIfTable2")
	pFreeMibTable   = iphlpapi.NewProc("FreeMibTable")
	pGetTickCount64 = kernel32t.NewProc("GetTickCount64")
)

// MIB_IF_ROW2 (netioapi.h): 1352 bytes; InterfaceIndex at 8, InOctets at 1208, OutOctets at 1280.
const (
	ifRow2Size      = 1352
	ifRow2Index     = 8
	ifRow2InOctets  = 1208
	ifRow2OutOctets = 1280
)

// ifCounters64 reads every adapter's 64-bit byte counters (these do not wrap at 4 GB like GetIfTable's).
func ifCounters64() map[int][2]uint64 {
	res := map[int][2]uint64{}
	var table uintptr
	if r, _, _ := pGetIfTable2.Call(uintptr(unsafe.Pointer(&table))); r != 0 || table == 0 {
		return res
	}
	defer pFreeMibTable.Call(table)
	base := unsafe.Add(nil, table) // memory owned by Windows until FreeMibTable
	n := *(*uint32)(base)
	for i := 0; i < int(n); i++ {
		row := unsafe.Add(base, 8+i*ifRow2Size)
		idx := *(*uint32)(unsafe.Add(row, ifRow2Index))
		in := *(*uint64)(unsafe.Add(row, ifRow2InOctets))
		out := *(*uint64)(unsafe.Add(row, ifRow2OutOctets))
		res[int(idx)] = [2]uint64{in, out}
	}
	return res
}

// bootTime is when Windows started (adapter counters start from zero then).
func bootTime() time.Time {
	ms, _, _ := pGetTickCount64.Call()
	return time.Now().Add(-time.Duration(ms) * time.Millisecond)
}
