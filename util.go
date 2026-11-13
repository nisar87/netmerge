package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	rootDir string
	logMu   sync.Mutex
	logFile *os.File
)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func initLog() {
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0o755)
	p := filepath.Join(rootDir, "data", "netmerge.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		logFile = f
	}
}

var logBytes int64

func logf(format string, a ...any) {
	line := fmt.Sprintf("%s  %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		_, _ = logFile.WriteString(line)
		logBytes += int64(len(line))
		if logBytes > 4<<20 { // keep the log small: one older copy, then start fresh
			p := logFile.Name()
			logFile.Close()
			_ = os.Rename(p, p+".old")
			if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
				logFile = f
			} else {
				logFile = nil
			}
			logBytes = 0
		}
	}
	fmt.Print(line)
}

// safely runs fn and turns a crash into a log line (and a crash report) instead of ending the whole app.
// It reports whether fn crashed.
func safely(what string, fn func()) (crashed bool) {
	defer func() {
		if r := recover(); r != nil {
			crashed = true
			logf("CRASH in %s (recovered): %v (%s)", what, r, where(3))
			pendingCrash(fmt.Sprintf("crash in %s (recovered, NetMerge kept running): %v\n%s", what, r, stackOfOurs()))
		}
	}()
	fn()
	return false
}

// keepRunning runs a loop that should never end; if it crashes it is started again (after a pause that grows if it
// keeps crashing), so one bad moment does not leave statistics or health checks stopped until NetMerge restarts.
func keepRunning(what string, fn func()) {
	pause := 5 * time.Second
	for safely(what, fn) {
		logf("%s: starting it again in %v", what, pause)
		time.Sleep(pause)
		if pause < 5*time.Minute {
			pause *= 2
		}
	}
}

// quoteArgs builds a Windows command line from separate arguments so that a folder with spaces (C:\Users\Jo Smith\x)
// stays one argument after the program restarts itself as administrator.
func quoteArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = quoteArg(a)
	}
	return strings.Join(out, " ")
}

// quoteArg follows the rules of the Windows command line (backslashes before a quote are doubled).
func quoteArg(a string) string {
	if a != "" && !strings.ContainsAny(a, " \t\"") {
		return a
	}
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range a {
		switch c {
		case '\\':
			slashes++
			b.WriteRune(c)
		case '"':
			b.WriteString(strings.Repeat("\\", slashes+1))
			b.WriteByte('"')
			slashes = 0
		default:
			slashes = 0
			b.WriteRune(c)
		}
	}
	b.WriteString(strings.Repeat("\\", slashes)) // a trailing backslash must not escape the closing quote
	b.WriteByte('"')
	return b.String()
}
