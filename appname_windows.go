//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modVersion            = syscall.NewLazyDLL("version.dll")
	pGetFileVersionInfoSz = modVersion.NewProc("GetFileVersionInfoSizeW")
	pGetFileVersionInfo   = modVersion.NewProc("GetFileVersionInfoW")
	pVerQueryValue        = modVersion.NewProc("VerQueryValueW")
)

// readFileDescription reads the "File description" (or else "Product name") from a program's version information,
// the name Windows' Task Manager shows for it.
func readFileDescription(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	size, _, _ := pGetFileVersionInfoSz.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 || size > 1<<20 {
		return ""
	}
	buf := make([]byte, size)
	if r, _, _ := pGetFileVersionInfo.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return ""
	}
	// VerQueryValue answers with a pointer into buf: turn it into an offset, so the bytes are read from buf itself
	base := uintptr(unsafe.Pointer(&buf[0]))
	query := func(q string) (int, int) {
		qp, _ := syscall.UTF16PtrFromString(q)
		var out uintptr
		var n uint32
		if r, _, _ := pVerQueryValue.Call(base, uintptr(unsafe.Pointer(qp)), uintptr(unsafe.Pointer(&out)), uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 || out < base || out >= base+size {
			return -1, 0
		}
		return int(out - base), int(n)
	}
	// the languages the file has names in; then the common English and neutral ones
	var langs []string
	if off, n := query(`\VarFileInfo\Translation`); off >= 0 {
		for i := 0; i+4 <= n && off+i+4 <= len(buf); i += 4 {
			lang := uint16(buf[off+i]) | uint16(buf[off+i+1])<<8
			cp := uint16(buf[off+i+2]) | uint16(buf[off+i+3])<<8
			langs = append(langs, fmt.Sprintf("%04x%04x", lang, cp))
		}
	}
	langs = append(langs, "040904b0", "040904e4", "000004b0")
	for _, field := range []string{"FileDescription", "ProductName"} {
		for _, l := range langs {
			off, n := query(`\StringFileInfo\` + l + `\` + field)
			if off < 0 {
				continue
			}
			u := make([]uint16, 0, n)
			for i := 0; i < n && off+2*i+1 < len(buf); i++ {
				u = append(u, uint16(buf[off+2*i])|uint16(buf[off+2*i+1])<<8)
			}
			if s := syscall.UTF16ToString(u); s != "" {
				return s
			}
		}
	}
	return ""
}
