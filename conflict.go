package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Some security / filtering programs install a network filter driver that crashes Windows (blue screen in
// NETIO.SYS) when traffic arrives through the engine's virtual adapter. Windows crash dumps from this PC
// showed AdGuard's driver as the one inspecting the crashing streams, so NetMerge refuses to run next to it.
type conflictDef struct {
	Name     string
	Driver   string   // kernel driver service name
	Services []string // user-mode services to stop
	Procs    []string // programs to close
}

var knownConflicts = []conflictDef{
	{Name: "AdGuard", Driver: "adgnetworkwfpdrv", Services: []string{"AdguardSvc"}, Procs: []string{"Adguard.exe", "AdguardSvc.exe"}},
}

func serviceRunning(name string) bool {
	out, err := hiddenCmd("sc.exe", "query", name).Output()
	return err == nil && strings.Contains(string(out), "RUNNING")
}

var (
	conflictMu    sync.Mutex
	conflictAt    time.Time
	conflictCache string
)

func conflictMessage(c conflictDef) string {
	return c.Name + " is running. Its network driver can crash Windows (blue screen) while NetMerge is merging. Close " + c.Name + " first, or press \"Close " + c.Name + "\"."
}

// detectConflict returns a user-facing message when a known-bad driver is active ("" = all clear).
var conflictBusy bool

// detectConflict returns the last result at once; a stale one is refreshed in the background, so the status call
// the window makes every second never waits for the service check.
func detectConflict() string {
	conflictMu.Lock()
	defer conflictMu.Unlock()
	if time.Since(conflictAt) >= 8*time.Second && !conflictBusy {
		conflictBusy = true
		go safely("conflict check", func() {
			msg := ""
			for _, c := range knownConflicts {
				if serviceRunning(c.Driver) {
					msg = conflictMessage(c)
					break
				}
			}
			conflictMu.Lock()
			conflictAt, conflictCache, conflictBusy = time.Now(), msg, false
			conflictMu.Unlock()
		})
	}
	return conflictCache
}

func forgetConflictCache() {
	conflictMu.Lock()
	conflictAt = time.Time{}
	conflictMu.Unlock()
}

// fixConflicts closes the conflicting programs and unloads their drivers. It needs administrator rights.
func fixConflicts() error {
	var problems []string
	for _, c := range knownConflicts {
		if !serviceRunning(c.Driver) {
			continue
		}
		for _, s := range c.Services {
			_ = hiddenCmd("sc.exe", "stop", s).Run()
		}
		for _, p := range c.Procs {
			killProcessByName(p)
		}
		_ = hiddenCmd("sc.exe", "stop", c.Driver).Run()
		ok := false
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			if !serviceRunning(c.Driver) {
				ok = true
				break
			}
		}
		if !ok {
			problems = append(problems, fmt.Sprintf("%s's driver is still loaded. Quit %s from its tray icon, or restart Windows.", c.Name, c.Name))
		}
	}
	forgetConflictCache()
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, " "))
	}
	return nil
}
