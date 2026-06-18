//go:build !windows
// +build !windows

package oslogs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestCollectorCollectDisabled(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "sys.log")
	if err := os.WriteFile(logFile, []byte("test line\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   1 * time.Second,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: false}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if data != nil {
		t.Fatalf("expected nil data when disabled")
	}
}

func TestCollectorCollectReadsFile(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "sys.log")
	content := "Jan  1 00:00:01 host app[123]: hello world\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   256,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected payload")
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) == 0 {
		t.Fatalf("expected events")
	}
	if payload.Events[0]["file"] == "" {
		t.Fatalf("expected file in event")
	}
	if payload.Events[0]["message"] == "" {
		t.Fatalf("expected message in event")
	}
	if payload.Events[0]["schema_version"] != float64(1) {
		t.Fatalf("expected schema_version in event, got %#v", payload.Events[0])
	}
	if payload.Events[0]["redaction_status"] == "" {
		t.Fatalf("expected redaction_status in event")
	}
}

func TestCollectorRedactsSensitiveValuesAndExtractsJSONAttributes(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := `{"level":"info","token":"abc123","message":"Authorization: Bearer secret-token password=hunter2"}`
	if err := os.WriteFile(logFile, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   512,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(payload.Events))
	}
	msg, _ := payload.Events[0]["message"].(string)
	if msg == "" || containsAny(msg, "abc123", "secret-token", "hunter2") {
		t.Fatalf("message leaked secret: %q", msg)
	}
	if payload.Events[0]["redaction_status"] != "redacted" {
		t.Fatalf("expected redacted status, got %#v", payload.Events[0]["redaction_status"])
	}
	attrs, ok := payload.Events[0]["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected attributes, got %#v", payload.Events[0])
	}
	if attrs["token"] != "[redacted]" {
		t.Fatalf("expected redacted token attribute, got %#v", attrs)
	}
}

func TestCollectorCombinesMultilineStackTrace(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		"Jan  1 00:00:01 host app[123]: panic: boom",
		"goroutine 1 [running]:",
		"main.main()",
		"Jan  1 00:00:02 host app[123]: recovered",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:      []string{logFile},
		OSLogCursorPath: filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines: 10,
		OSLogMaxBytes:   1024,
		OSLogInterval:   1 * time.Second,
		OSLogEnrich:     true,
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 2 {
		t.Fatalf("expected 2 multiline events, got %d: %#v", len(payload.Events), payload.Events)
	}
	msg, _ := payload.Events[0]["message"].(string)
	if !strings.Contains(msg, "goroutine 1") || !strings.Contains(msg, "main.main()") {
		t.Fatalf("expected stack trace in first event, got %q", msg)
	}
}

func TestCollectorAppliesFiltersWithoutPersistingDroppedContent(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "app.log")
	content := strings.Join([]string{
		"Jan  1 00:00:01 host app[123]: health check ok",
		"Jan  1 00:00:02 host app[123]: failed login password=secret",
	}, "\n") + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write log file: %v", err)
	}
	cfg := config.Config{
		OSLogFiles:        []string{logFile},
		OSLogCursorPath:   filepath.Join(tmp, "cursor.json"),
		OSLogBatchLines:   10,
		OSLogMaxBytes:     1024,
		OSLogInterval:     1 * time.Second,
		OSLogEnrich:       true,
		OSLogExcludeRegex: "health check",
	}
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{OSLogFiles: true, OSLogEnrich: true}
	}

	c := New(cfg, prefs)
	data, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	var payload struct {
		Events       []map[string]any `json:"events"`
		DroppedCount int              `json:"dropped_count"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 event after filter, got %d: %#v", len(payload.Events), payload.Events)
	}
	if payload.DroppedCount != 1 {
		t.Fatalf("expected dropped_count=1, got %d", payload.DroppedCount)
	}
	msg, _ := payload.Events[0]["message"].(string)
	if strings.Contains(msg, "secret") || strings.Contains(msg, "health check") {
		t.Fatalf("unexpected leaked or dropped content: %q", msg)
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
