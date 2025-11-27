//go:build !windows
// +build !windows

package oslogs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type collector struct {
	files      []string
	cursorPath string
	batchLines int
	maxBytes   int
	cursor     map[string]int64
	interval   time.Duration
}

func New(cfg config.Config) ports.Collector {
	return &collector{
		files:      cfg.OSLogFiles,
		cursorPath: cfg.OSLogCursorPath,
		batchLines: cfg.OSLogBatchLines,
		maxBytes:   cfg.OSLogMaxBytes,
		cursor:     loadCursor(cfg.OSLogCursorPath),
		interval:   cfg.OSLogInterval,
	}
}

func (c *collector) Name() string { return "oslogs" }

func (c *collector) Interval() time.Duration { return c.interval }

type logEvent struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source,omitempty"`
	File      string `json:"file"`
	Message   string `json:"message"`
}

type payload struct {
	Events []logEvent `json:"events"`
}

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	if len(c.files) == 0 {
		return nil, nil
	}
	hostname, _ := os.Hostname()
	var events []logEvent
	for _, path := range c.files {
		evs := c.readFile(path, hostname)
		events = append(events, evs...)
		if len(events) >= c.batchLines {
			break
		}
	}
	if len(events) == 0 {
		return nil, nil
	}
	_ = saveCursor(c.cursorPath, c.cursor)
	return json.Marshal(payload{Events: events})
}

func (c *collector) readFile(path, hostname string) []logEvent {
	var out []logEvent
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	offset := c.cursor[path]
	if offset > 0 {
		_, _ = f.Seek(offset, 0)
	}
	r := bufio.NewReader(f)
	for len(out) < c.batchLines {
		line, err := r.ReadString('\n')
		if line != "" {
			if len(line) > c.maxBytes {
				line = line[:c.maxBytes]
			}
			out = append(out, logEvent{
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Source:    hostname,
				File:      path,
				Message:   line,
			})
		}
		if err != nil {
			break
		}
	}
	if pos, err := f.Seek(0, 1); err == nil {
		c.cursor[path] = pos
	}
	return out
}

func loadCursor(path string) map[string]int64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]int64{}
	}
	var cur map[string]int64
	if err := json.Unmarshal(b, &cur); err != nil {
		return map[string]int64{}
	}
	return cur
}

func saveCursor(path string, cur map[string]int64) error {
	if path == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	raw, _ := json.Marshal(cur)
	return os.WriteFile(path, raw, 0o600)
}
