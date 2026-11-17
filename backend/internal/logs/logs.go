// Package logs mirrors the process log stream into an in-memory ring buffer so
// the admin console can display live server output. Captured lines are still
// written to the original console writer unchanged.
package logs

import (
	"bytes"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry is one captured console line.
type Entry struct {
	Seq     int64  `json:"seq"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

const (
	maxEntries       = 600
	subscriberBuffer = 256
)

var state = struct {
	sync.Mutex
	entries     []Entry
	nextSeq     int64
	subscribers map[chan Entry]struct{}
	dropped     atomic.Int64
}{subscribers: map[chan Entry]struct{}{}}

// Install redirects the standard logger through the capture writer while
// keeping console output intact.
func Install(console io.Writer) {
	if console == nil {
		console = os.Stderr
	}
	log.SetOutput(&captureWriter{console: console})
}

// Recent returns at most limit entries, optionally only those newer than
// afterSeq. Entries are returned oldest first.
func Recent(limit int, afterSeq int64) []Entry {
	if limit <= 0 || limit > maxEntries {
		limit = 200
	}
	state.Lock()
	defer state.Unlock()
	start := 0
	if afterSeq > 0 {
		// Entries are ordered by sequence; keep only newer ones.
		for start < len(state.entries) && state.entries[start].Seq <= afterSeq {
			start++
		}
	}
	entries := state.entries[start:]
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return append([]Entry(nil), entries...)
}

// Subscribe returns a channel receiving every new entry and a cancel function
// that must be called exactly once.
func Subscribe() (<-chan Entry, func()) {
	ch := make(chan Entry, subscriberBuffer)
	state.Lock()
	state.subscribers[ch] = struct{}{}
	state.Unlock()
	cancel := func() {
		state.Lock()
		if _, ok := state.subscribers[ch]; ok {
			delete(state.subscribers, ch)
			close(ch)
		}
		state.Unlock()
	}
	return ch, cancel
}

// Dropped reports how many entries were discarded because a subscriber could
// not keep up. It is diagnostic only.
func Dropped() int64 { return state.dropped.Load() }

func publish(entry Entry) {
	state.Lock()
	state.nextSeq++
	entry.Seq = state.nextSeq
	state.entries = append(state.entries, entry)
	// Trim in batches so the ring does not copy the whole buffer per line.
	if len(state.entries) > maxEntries+maxEntries/4 {
		state.entries = append([]Entry(nil), state.entries[len(state.entries)-maxEntries:]...)
	}
	subscribers := make([]chan Entry, 0, len(state.subscribers))
	for ch := range state.subscribers {
		subscribers = append(subscribers, ch)
	}
	state.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- entry:
		default:
			state.dropped.Add(1)
		}
	}
}

type captureWriter struct {
	mu      sync.Mutex
	console io.Writer
	pending []byte
}

func (w *captureWriter) Write(chunk []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.console.Write(chunk)
	w.pending = append(w.pending, chunk...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := string(w.pending[:index])
		w.pending = w.pending[index+1:]
		if line = strings.TrimSuffix(line, "\r"); line != "" {
			publish(parseLine(line))
		}
	}
	// A write without a trailing newline should not be lost forever.
	if len(w.pending) > 64*1024 {
		publish(parseLine(string(w.pending)))
		w.pending = nil
	}
	return len(chunk), nil
}

func parseLine(line string) Entry {
	entry := Entry{Time: time.Now().Format(time.RFC3339), Message: line}
	if when, rest, ok := splitLogPrefix(line); ok {
		entry.Time = when.Format(time.RFC3339)
		entry.Message = rest
	}
	entry.Level = detectLevel(entry.Message)
	return entry
}

// splitLogPrefix strips the standard "2006/01/02 15:04:05 " prefix emitted by
// the default log flags so the console can render its own timestamp column.
func splitLogPrefix(line string) (time.Time, string, bool) {
	const prefixLen = len("2006/01/02 15:04:05 ")
	if len(line) < prefixLen {
		return time.Time{}, line, false
	}
	when, err := time.ParseInLocation("2006/01/02 15:04:05", line[:prefixLen-1], time.Local)
	if err != nil {
		return time.Time{}, line, false
	}
	return when, line[prefixLen:], true
}

func detectLevel(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(message, "错误"),
		strings.Contains(message, "失败"),
		strings.Contains(lower, "error"),
		strings.Contains(lower, "fatal"),
		strings.Contains(lower, "failed"):
		return "error"
	case strings.Contains(message, "警告"),
		strings.Contains(message, "告警"),
		strings.Contains(lower, "warn"):
		return "warn"
	default:
		return "info"
	}
}
