package logger

import (
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/redaction"
)

const SupportLogBytes = 16 * 1024
const SupportLogLines = 100
const SupportLogAge = time.Hour

type supportEntry struct {
	at   time.Time
	text string
}

// SupportBuffer retains only redacted process-owned logs in bounded volatile memory.
// It never grants access to other services or persists another log file.
type SupportBuffer struct {
	delegate  Logger
	mu        sync.Mutex
	entries   []supportEntry
	bytes     int
	truncated bool
	now       func() time.Time
}

func WithSupportBuffer(delegate Logger) *SupportBuffer {
	return &SupportBuffer{delegate: delegate, now: time.Now}
}

func (b *SupportBuffer) Info(msg string)             { b.record("INFO", msg); b.delegate.Info(msg) }
func (b *SupportBuffer) Error(msg string)            { b.record("ERROR", msg); b.delegate.Error(msg) }
func (b *SupportBuffer) Fatal(msg string, kv ...any) { b.delegate.Fatal(msg, kv...) }
func (b *SupportBuffer) Sync()                       { b.delegate.Sync() }

func (b *SupportBuffer) record(level, msg string) {
	now := b.now().UTC()
	// Redact before truncating: a credential marker after the byte limit must still
	// remove its entire line, rather than leave an unsafe prefix in the buffer.
	text := now.Format(time.RFC3339Nano) + " " + level + " " + redaction.SupportText(msg) + "\n"
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(now)
	if len(text) > SupportLogBytes {
		text = text[:SupportLogBytes]
		b.truncated = true
	}
	b.entries = append(b.entries, supportEntry{now, text})
	b.bytes += len(text)
	for len(b.entries) > SupportLogLines || b.bytes > SupportLogBytes {
		b.bytes -= len(b.entries[0].text)
		b.entries[0] = supportEntry{}
		b.entries = b.entries[1:]
		b.truncated = true
	}
}

func (b *SupportBuffer) prune(now time.Time) {
	for len(b.entries) > 0 && now.Sub(b.entries[0].at) > SupportLogAge {
		b.bytes -= len(b.entries[0].text)
		b.entries[0] = supportEntry{}
		b.entries = b.entries[1:]
	}
}

func (b *SupportBuffer) SupportLog() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prune(b.now().UTC())
	var text strings.Builder
	for _, entry := range b.entries {
		text.WriteString(entry.text)
	}
	return text.String(), b.truncated
}
