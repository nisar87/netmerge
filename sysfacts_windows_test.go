//go:build windows

package main

import (
	"strings"
	"testing"
)

// the diagnostics report names Windows and the PC; on a real Windows machine both must be read
func TestSysFacts(t *testing.T) {
	f := readSysFacts()
	if !strings.Contains(f["os"], "Windows") || f["build"] == "" || f["memory"] == "" {
		t.Fatalf("facts: %v", f)
	}
	t.Logf("%v", f)
	if b := bootTime(); b.IsZero() {
		t.Fatal("no boot time")
	}
}
