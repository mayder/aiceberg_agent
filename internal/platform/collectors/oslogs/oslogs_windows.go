//go:build windows
// +build windows

package oslogs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type winCollector struct {
	channels   []string
	cursorPath string
	cursor     map[string]uint64
	batchLines int
	maxBytes   int
	interval   time.Duration
}

type logEvent struct {
	Timestamp string `json:"timestamp"`
	Source    string `json:"source,omitempty"`
	Channel   string `json:"channel,omitempty"`
	EventID   uint64 `json:"event_id,omitempty"`
	RecordID  uint64 `json:"record_id,omitempty"`
	Level     string `json:"level,omitempty"`
	Computer  string `json:"computer,omitempty"`
	Message   string `json:"message"`
}

type payload struct {
	Events []logEvent `json:"events"`
}

func New(cfg config.Config) ports.Collector {
	ch := cfg.OSLogWinChannels
	if len(ch) == 0 {
		ch = []string{"Security", "System", "Application", "Microsoft-Windows-Sysmon/Operational"}
	}
	return &winCollector{
		channels:   ch,
		cursorPath: cfg.OSLogCursorPath,
		cursor:     loadCursorWin(cfg.OSLogCursorPath),
		batchLines: cfg.OSLogBatchLines,
		maxBytes:   cfg.OSLogMaxBytes,
		interval:   cfg.OSLogInterval,
	}
}

func (c *winCollector) Name() string { return "oslogs" }

func (c *winCollector) Interval() time.Duration { return c.interval }

func (c *winCollector) Collect(ctx context.Context) ([]byte, error) {
	hostname, _ := os.Hostname()
	var out []logEvent

	for _, ch := range c.channels {
		if len(out) >= c.batchLines {
			break
		}
		last := c.cursor[ch]
		events := c.fetchChannel(ctx, ch, last, c.batchLines-len(out), hostname)
		if len(events) > 0 {
			out = append(out, events...)
			maxRec := last
			for _, ev := range events {
				if ev.RecordID > maxRec {
					maxRec = ev.RecordID
				}
			}
			c.cursor[ch] = maxRec
		}
	}

	if len(out) == 0 {
		return nil, nil
	}
	_ = saveCursorWin(c.cursorPath, c.cursor)
	return json.Marshal(payload{Events: out})
}

func (c *winCollector) fetchChannel(ctx context.Context, channel string, lastRecord uint64, limit int, hostname string) []logEvent {
	var events []logEvent
	query := "*[System[EventRecordID>" + strconv.FormatUint(lastRecord, 10) + "]]"
	args := []string{"qe", channel, "/q:" + query, "/f:Text", "/c:" + strconv.Itoa(limit), "/rd:true"}
	cmd := exec.CommandContext(ctx, "wevtutil", args...)
	raw, err := cmd.Output()
	if err != nil {
		return events
	}
	blocks := splitEvents(string(raw))
	for _, blk := range blocks {
		ev := parseEventBlock(blk, channel, hostname, c.maxBytes)
		if ev.RecordID == 0 {
			continue
		}
		events = append(events, ev)
	}
	return events
}

func splitEvents(s string) []string {
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(s))
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Event[") {
			if buf.Len() > 0 {
				out = append(out, buf.String())
				buf.Reset()
			}
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if buf.Len() > 0 {
		out = append(out, buf.String())
	}
	return out
}

func parseEventBlock(block, channel, hostname string, maxBytes int) logEvent {
	ev := logEvent{Channel: channel, Source: hostname, Timestamp: time.Now().UTC().Format(time.RFC3339Nano)}
	lines := strings.Split(block, "\n")
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "Event ID:") {
			if id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(ln, "Event ID:")), 10, 64); err == nil {
				ev.EventID = id
			}
		} else if strings.HasPrefix(ln, "Record ID:") {
			if id, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(ln, "Record ID:")), 10, 64); err == nil {
				ev.RecordID = id
			}
		} else if strings.HasPrefix(ln, "Level:") {
			ev.Level = strings.TrimSpace(strings.TrimPrefix(ln, "Level:"))
		} else if strings.HasPrefix(ln, "Computer:") {
			ev.Computer = strings.TrimSpace(strings.TrimPrefix(ln, "Computer:"))
		}
	}
	msg := block
	if len(msg) > maxBytes {
		msg = msg[:maxBytes]
	}
	ev.Message = msg
	return ev
}

func loadCursorWin(path string) map[string]uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]uint64{}
	}
	var cur map[string]uint64
	if err := json.Unmarshal(b, &cur); err != nil {
		return map[string]uint64{}
	}
	return cur
}

func saveCursorWin(path string, cur map[string]uint64) error {
	if path == "" {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	raw, _ := json.Marshal(cur)
	return os.WriteFile(path, raw, 0o600)
}
