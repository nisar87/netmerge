package engine

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Logger writes lines in the form NetMerge reads (and mihomo writes):
//
//	time="2026-11-12T03:12:11.207397100+05:00" level=info msg="[TCP] 1.2.3.4:5(app.exe) --> host:443 match ... using ..."
type Logger struct {
	mu    sync.Mutex
	w     io.Writer
	level atomic.Int32 // 0 debug, 1 info, 2 warning, 3 error, 4 silent
}

func levelOf(s string) int {
	switch strings.ToLower(s) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warning", "warn":
		return 2
	case "error":
		return 3
	case "silent":
		return 4
	}
	return 1
}

func NewLogger(w io.Writer, level string) *Logger {
	l := &Logger{w: w}
	l.level.Store(int32(levelOf(level)))
	return l
}

func (l *Logger) SetLevel(level string) {
	l.level.Store(int32(levelOf(level)))
}

func (l *Logger) enabled(lv int) bool { return int32(lv) >= l.level.Load() }

func (l *Logger) write(lv int, name, format string, a ...any) {
	if l == nil || !l.enabled(lv) {
		return
	}
	msg := fmt.Sprintf(format, a...)
	msg = strings.ReplaceAll(strings.ReplaceAll(msg, `\`, `\\`), `"`, `\"`)
	line := fmt.Sprintf("time=%q level=%s msg=\"%s\"\n", time.Now().Format(time.RFC3339Nano), name, msg)
	l.mu.Lock()
	_, _ = io.WriteString(l.w, line)
	l.mu.Unlock()
}

func (l *Logger) Debugf(f string, a ...any) { l.write(0, "debug", f, a...) }
func (l *Logger) Infof(f string, a ...any)  { l.write(1, "info", f, a...) }
func (l *Logger) Warnf(f string, a ...any)  { l.write(2, "warning", f, a...) }
func (l *Logger) Errorf(f string, a ...any) { l.write(3, "error", f, a...) }
