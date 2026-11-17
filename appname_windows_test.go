//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Programs that come with Windows carry a name in their version information; NetMerge shows it in Apps.
func TestReadFileDescription(t *testing.T) {
	p := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	name := readFileDescription(p)
	t.Logf("%s: %q", p, name)
	if !strings.Contains(strings.ToLower(name), "notepad") {
		t.Fatalf("name of notepad.exe: %q", name)
	}
	if readFileDescription(filepath.Join(t.TempDir(), "missing.exe")) != "" {
		t.Fatal("a missing file has no name")
	}
}
