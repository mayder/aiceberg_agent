package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestBootstrapSupportFlareCollectsRealBoundedToolOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixed tool-selection shell fixture requires POSIX; native Windows pilot is separate")
	}
	for _, platform := range []string{"linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			directory := t.TempDir()
			tool := "journalctl"
			if platform == "windows" {
				tool = "wevtutil"
			}
			script := "#!/bin/sh\nprintf '%s\\n' 'service failed' 'password=private token=another'\n"
			if err := os.WriteFile(filepath.Join(directory, tool), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", directory)
			log := logger.New("error")
			defer log.Sync()
			executor := usecase.NewSelfHealExecutor(log, nil, usecase.SelfHealDeps{SupportFlare: newSupportFlare(platform)})
			status, _, evidence := executor.Execute(context.Background(), entities.SelfHealCommand{CommandID: "support-bootstrap", Code: "collect_support_flare"})
			if status != "success" {
				t.Fatal(status, evidence)
			}
			data, _ := json.Marshal(evidence)
			if !strings.Contains(string(data), "service failed") || strings.Contains(string(data), "private") || strings.Contains(string(data), "another") {
				t.Fatal(string(data))
			}
		})
	}
}

func TestBootstrapSupportWithoutToolFailsWithEvidence(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result, err := newSupportFlare("linux")(context.Background())
	if err == nil || result["status"] != "unavailable" {
		t.Fatal(result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = newSupportFlare("linux")(ctx); err == nil {
		t.Fatal("cancelled support must fail")
	}
}

func TestBootstrapSupportUsesOnlyProcessLogsWhenJournalUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	buffered := logger.WithSupportBuffer(logger.New("error"))
	defer buffered.Sync()
	buffered.Info("real process diagnostic")
	buffered.Error("password=hidden token=another")
	result, err := newSupportFlare("linux", buffered)(context.Background())
	if err != nil || result["status"] != "collected" || result["degraded"] != true {
		t.Fatal(result, err)
	}
	rows := result["sources"].([]any)
	first := rows[0].(map[string]any)
	last := rows[len(rows)-1].(map[string]any)
	if first["status"] != "missing" || last["source"] != "agent_service_process_log" || last["volatile"] != true {
		t.Fatal(rows)
	}
	data, _ := json.Marshal(rows)
	if !strings.Contains(string(data), "real process diagnostic") || strings.Contains(string(data), "hidden") || strings.Contains(string(data), "another") {
		t.Fatal(string(data))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = newSupportFlare("linux", buffered)(ctx); err == nil {
		t.Fatal("cancelled collection succeeded")
	}
	if _, err = newSupportFlare("darwin", buffered)(context.Background()); err == nil {
		t.Fatal("unsupported platform succeeded")
	}
	empty := logger.WithSupportBuffer(logger.New("error"))
	defer empty.Sync()
	if _, err = newSupportFlare("linux", empty)(context.Background()); err == nil {
		t.Fatal("empty process log succeeded")
	}
}
