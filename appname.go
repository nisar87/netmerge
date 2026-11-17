package main

import (
	"path/filepath"
	"strings"
	"sync"
)

// Friendly app names: the name an app gives itself in its program file ("Microsoft Teams" for ms-teams.exe),
// read once per program file in the background and kept. The Apps page shows it, with the process name below.
var (
	appNameMu   sync.Mutex
	appNameByP  = map[string]string{} // program file → its name ("" when it has none)
	appNameProc = map[string]string{} // process (lower case) → its name, for the rules list
	appNameBusy = map[string]bool{}
)

// appName returns the name known for a program file, starting a lookup the first time it is asked.
func appName(proc, path string) string {
	if path == "" {
		return ""
	}
	appNameMu.Lock()
	n, ok := appNameByP[path]
	if !ok && !appNameBusy[path] {
		appNameBusy[path] = true
		go safely("read an app's name", func() {
			name := cleanAppName(readFileDescription(path), proc)
			appNameMu.Lock()
			appNameByP[path] = name
			delete(appNameBusy, path)
			if name != "" && len(appNameProc) < 2000 {
				appNameProc[strings.ToLower(proc)] = name
			}
			appNameMu.Unlock()
		})
	}
	appNameMu.Unlock()
	return n
}

// appNames is every process name known so far, for the window (rules of apps not running now too).
func appNames() map[string]string {
	appNameMu.Lock()
	defer appNameMu.Unlock()
	m := make(map[string]string, len(appNameProc))
	for k, v := range appNameProc {
		m[k] = v
	}
	return m
}

// cleanAppName drops descriptions that say nothing ("", the file name again, "Application") and trims long ones.
func cleanAppName(name, proc string) string {
	name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
	base := strings.TrimSuffix(strings.ToLower(filepath.Base(proc)), ".exe")
	switch l := strings.ToLower(name); {
	case name == "", l == base, l == base+".exe", l == "application", l == "app", l == "program":
		return ""
	}
	if r := []rune(name); len(r) > 48 {
		name = string(r[:47]) + "…"
	}
	return name
}
