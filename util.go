package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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

// safely runs fn and turns a crash into a log line instead of ending the whole app.
func safely(what string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			logf("recovered from a crash in %s: %v", what, r)
		}
	}()
	fn()
}
