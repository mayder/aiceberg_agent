//go:build !windows
// +build !windows

package oslogs

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

const journaldCursorPrefix = "journald:"

type journalRunner func(context.Context, []string) ([]byte, error)

type journalReader struct {
	enabled    bool
	units      []string
	priorities []string
	run        journalRunner
}

func newJournalReader(enabled bool, units, priorities []string, run journalRunner) journalReader {
	return journalReader{
		enabled:    enabled,
		units:      sanitizeJournalUnits(units),
		priorities: sanitizeJournalPriorities(priorities),
		run:        run,
	}
}

func (j *journalReader) applyPrefs(p config.CollectPrefs) {
	if p.OSLogJournaldEnabled {
		j.enabled = true
	}
	if len(p.OSLogJournaldUnits) > 0 {
		j.units = sanitizeJournalUnits(p.OSLogJournaldUnits)
	}
	if len(p.OSLogJournaldPriorities) > 0 {
		j.priorities = sanitizeJournalPriorities(p.OSLogJournaldPriorities)
	}
}

func (j journalReader) read(ctx context.Context, cursor map[string]int64, hostname string, limit int, maxBytes int) []logEvent {
	if !j.enabled || limit <= 0 || j.run == nil {
		return nil
	}
	key := j.cursorKey()
	args := j.args(cursor[key], limit)
	raw, err := j.run(ctx, args)
	if err != nil {
		return nil
	}
	events := parseJournalLines(raw, hostname, maxBytes, j.priorities, cursor[key])
	if len(events) > limit {
		events = events[:limit]
	}
	for _, ev := range events {
		if ts, ok := journalTimestampMicros(ev); ok && ts > cursor[key] {
			cursor[key] = ts
		}
	}
	return events
}

func (j journalReader) args(lastMicros int64, limit int) []string {
	args := []string{"--output=json", "--no-pager", "-n", strconv.Itoa(limit)}
	if lastMicros > 0 {
		args = append(args, "--since", "@"+strconv.FormatInt(lastMicros/int64(time.Second/time.Microsecond), 10))
	}
	for _, unit := range j.units {
		args = append(args, "-u", unit)
	}
	return args
}

func (j journalReader) cursorKey() string {
	return journaldCursorPrefix + strings.Join(j.units, ",") + "|" + strings.Join(j.priorities, ",")
}

func runJournalctl(ctx context.Context, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	return cmd.Output()
}

func parseJournalLines(raw []byte, hostname string, maxBytes int, priorities []string, lastMicros int64) []logEvent {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	events := make([]logEvent, 0, len(lines))
	for _, line := range lines {
		ev, ok := parseJournalLine(line, hostname, maxBytes)
		if !ok || !journalPriorityAllowed(ev, priorities) {
			continue
		}
		if ts, ok := journalTimestampMicros(ev); ok && ts <= lastMicros {
			continue
		}
		events = append(events, ev)
	}
	return events
}

func parseJournalLine(line string, hostname string, maxBytes int) (logEvent, bool) {
	var item map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &item); err != nil {
		return logEvent{}, false
	}
	message := journalString(item, "MESSAGE")
	if message == "" {
		return logEvent{}, false
	}
	if len(message) > maxBytes {
		message = message[:maxBytes]
	}
	message, redactionStatus := redactMessage(message)
	priority := journalString(item, "PRIORITY")
	timestamp := journalTimestamp(item)
	unit := journalString(item, "_SYSTEMD_UNIT")
	host := journalString(item, "_HOSTNAME")
	if host == "" {
		host = hostname
	}
	attrs := journalAttributes(item)
	ev := logEvent{
		SchemaVersion:   logSchemaVersion,
		Timestamp:       timestamp.Format(time.RFC3339Nano),
		TimestampUTC:    timestamp.UTC().Format(time.RFC3339Nano),
		Source:          host,
		Host:            host,
		File:            "journald",
		Path:            "journald:" + unit,
		Cursor:          journalString(item, "_SOURCE_REALTIME_TIMESTAMP"),
		Message:         message,
		App:             journalString(item, "SYSLOG_IDENTIFIER"),
		Service:         unit,
		PID:             journalString(item, "_PID"),
		Level:           journalPriorityName(priority),
		Severity:        journalPriorityName(priority),
		Attributes:      attrs,
		RedactionStatus: redactionStatus,
		Transport:       "agent_journald",
		SourceTool:      "journald",
		SourceCategory:  "observability",
	}
	return ev, true
}

func journalAttributes(item map[string]any) map[string]any {
	attrs := map[string]any{}
	for _, key := range []string{"_SYSTEMD_UNIT", "SYSLOG_IDENTIFIER", "PRIORITY", "_PID", "_COMM"} {
		if value := journalString(item, key); value != "" {
			attrs[key] = value
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func journalString(item map[string]any, key string) string {
	switch value := item[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatInt(int64(value), 10)
	default:
		return ""
	}
}

func journalTimestamp(item map[string]any) time.Time {
	raw := journalString(item, "_SOURCE_REALTIME_TIMESTAMP")
	if micros, err := strconv.ParseInt(raw, 10, 64); err == nil && micros > 0 {
		return time.UnixMicro(micros).UTC()
	}
	return time.Now().UTC()
}

func journalTimestampMicros(ev logEvent) (int64, bool) {
	micros, err := strconv.ParseInt(ev.Cursor, 10, 64)
	return micros, err == nil && micros > 0
}

func journalPriorityAllowed(ev logEvent, priorities []string) bool {
	if len(priorities) == 0 {
		return true
	}
	for _, priority := range priorities {
		if priority == ev.Level || priority == journalPriorityNumber(ev.Level) {
			return true
		}
	}
	return false
}

func journalPriorityName(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "0":
		return "emergency"
	case "1":
		return "alert"
	case "2":
		return "critical"
	case "3":
		return "error"
	case "4":
		return "warning"
	case "5":
		return "notice"
	case "6":
		return "info"
	case "7":
		return "debug"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

func journalPriorityNumber(name string) string {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "emergency", "emerg":
		return "0"
	case "alert":
		return "1"
	case "critical", "crit":
		return "2"
	case "error", "err":
		return "3"
	case "warning", "warn":
		return "4"
	case "notice":
		return "5"
	case "info", "information":
		return "6"
	case "debug", "trace":
		return "7"
	default:
		return ""
	}
}

func sanitizeJournalUnits(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean != "" && isSafeJournalToken(clean) {
			out = append(out, clean)
		}
	}
	return out
}

func sanitizeJournalPriorities(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		clean := strings.ToLower(strings.TrimSpace(value))
		if clean == "" {
			continue
		}
		if canonical := canonicalJournalPriority(clean); canonical != "" {
			out = append(out, canonical)
		}
	}
	return out
}

func canonicalJournalPriority(value string) string {
	switch value {
	case "0", "1", "2", "3", "4", "5", "6", "7":
		return value
	case "emerg":
		return "emergency"
	case "crit":
		return "critical"
	case "err":
		return "error"
	case "warn":
		return "warning"
	case "information":
		return "info"
	case "trace":
		return "debug"
	case "emergency", "alert", "critical", "error", "warning", "notice", "info", "debug":
		return value
	default:
		return ""
	}
}

func isSafeJournalToken(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if strings.ContainsRune("._@:-", r) {
			continue
		}
		return false
	}
	return true
}
