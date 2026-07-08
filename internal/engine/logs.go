package engine

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// logBuffer is an in-memory ring of keen-manager's own log lines. Each appended
// line is also fanned out on the bus as an EventLog frame so the Logs page can
// stream it live. Service logs for xray/nfqws/awg are read from their files on
// demand (see tailFile).
type logBuffer struct {
	mu   sync.Mutex
	ring []string
	max  int
	bus  *bus
}

func newLogBuffer(max int, b *bus) *logBuffer {
	if max <= 0 {
		max = 500
	}
	return &logBuffer{ring: make([]string, 0, max), max: max, bus: b}
}

// appendf formats and records a keen-manager log line.
func (l *logBuffer) appendf(format string, args ...any) {
	l.append(fmt.Sprintf(format, args...))
}

func (l *logBuffer) append(line string) {
	stamped := time.Now().Format("15:04:05") + " " + line
	l.mu.Lock()
	l.ring = append(l.ring, stamped)
	if len(l.ring) > l.max {
		l.ring = l.ring[len(l.ring)-l.max:]
	}
	l.mu.Unlock()

	// Mirror to stderr so `logread`/init-script capture works on-device.
	fmt.Fprintln(os.Stderr, "[keen-manager] "+line)

	if l.bus != nil {
		l.bus.publish(Event{Type: EventLog, Data: map[string]string{
			"service": "keen-manager",
			"line":    stamped,
		}})
	}
}

// tail returns the last n keen-manager lines from the ring.
func (l *logBuffer) tail(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n <= 0 || n > len(l.ring) {
		n = len(l.ring)
	}
	out := make([]string, n)
	copy(out, l.ring[len(l.ring)-n:])
	return out
}

// tailFile returns the last n lines of a log file (best-effort; empty on error).
func tailFile(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Simple bounded read: keep a sliding window of the last n lines. Log files
	// on a router are small, so a full scan is fine and avoids seek complexity.
	var window []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		window = append(window, strings.TrimRight(sc.Text(), "\r\n"))
		if len(window) > n {
			window = window[len(window)-n:]
		}
	}
	return window
}
