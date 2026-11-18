//go:build windows

package main

import (
	"regexp"
	"testing"
	"unsafe"
)

func TestGUIDTextMatchesAdapterNames(t *testing.T) {
	g := [16]byte{0x78, 0x56, 0x34, 0x12, 0xbc, 0x9a, 0xf0, 0xde, 1, 2, 3, 4, 5, 6, 7, 8}
	if got := guidText(unsafe.Pointer(&g[0])); got != "{12345678-9ABC-DEF0-0102-030405060708}" {
		t.Fatalf("got %s", got)
	}
	// every adapter Windows lists has an id in the same form, so Wi-Fi interfaces can be matched to it
	list, err := listAdaptersAPI()
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?i)^\{[0-9A-F-]{36}\}$`)
	for _, a := range list {
		if !re.MatchString(a.GUID) {
			t.Fatalf("%s: id %q", a.Name, a.GUID)
		}
	}
	wifiNames() // must not crash on a machine without Wi-Fi
}
