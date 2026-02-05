//go:build !windows
// +build !windows

package oslogs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
}
