package usecase

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestUpdateReadinessCanonicalLaunchers(t *testing.T) {
	cases := []struct {
		os, command string
		valid       bool
	}{
		{"linux", "sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh", true},
		{"linux", "/usr/local/sbin/aiceberg-agent-update-launcher.sh", true},
		{"darwin", "/Library/AIceberg/sbin/aiceberg-agent-update-launcher.sh", true},
		{"darwin", "sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh", false},
		{"windows", `& 'C:\ProgramData\AIceberg\update\aiceberg-agent-update-launcher.ps1'`, true},
		{"windows", `& 'C:\Program Files\AIceberg\update\aiceberg-agent-update-launcher.ps1'`, true},
		{"linux", "sudo /usr/local/sbin/aiceberg-agent-update-launcher.sh", false},
		{"linux", "sudo -n /usr/local/sbin/aiceberg-agent-update-launcher.sh; echo bad", false},
		{"linux", "/tmp/aiceberg-agent-update-launcher.sh", false},
		{"freebsd", "/usr/local/sbin/aiceberg-agent-update-launcher.sh", false},
		{"windows", `& 'C:\update\aiceberg-agent-update-launcher.ps1'; whoami`, false},
	}
	for _, tc := range cases {
		if _, valid := officialUpdateLauncher(tc.command, tc.os); valid != tc.valid {
			t.Errorf("unexpected validity for %s: %q", tc.os, tc.command)
		}
	}
}

func TestUpdateReadinessRequiresEveryBooleanCheck(t *testing.T) {
	checks := map[string]bool{"enabled": true, "signed_updates_required": true, "config_version_known": true, "context_active": true,
		"local_preflight": true, "official_launcher": true, "apply_script": true, "service_manager": true, "service_authority": true,
		"no_pending_update": true, "config_unchanged": true}
	if !readinessChecksPassed(checks) {
		t.Fatal("valid checks rejected")
	}
	for name := range checks {
		checks[name] = false
		if readinessChecksPassed(checks) {
			t.Fatalf("missing %s accepted", name)
		}
		checks[name] = true
	}
	if readinessChecksPassed(map[string]bool{"enabled": true}) {
		t.Fatal("partial checks accepted")
	}
}

func TestUpdateReadinessDisabledDoesNotTouchFilesystem(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uncreated")
	uc := NewSelfUpdate(config.Config{AutoUpdateDir: dir}, &fakeLogger{})
	result := uc.CheckUpdateReadiness(context.Background())
	if result["ready"] != false || result["schema"] != UpdateReadinessSchema {
		t.Fatal("disabled updater accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("disabled check touched staging")
	}
	raw, _ := json.Marshal(result)
	if strings.Contains(string(raw), dir) {
		t.Fatal("result exposes a filesystem path")
	}
}

func TestUpdateReadinessPreservesPendingStateAndChecksConfigDrift(t *testing.T) {
	dir := t.TempDir()
	pending := filepath.Join(dir, ".pending_update.json")
	data := []byte(`{"target_version":"1.2.3","from_version":"1.0.0"}`)
	if err := os.WriteFile(pending, data, 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	uc := NewSelfUpdate(config.Config{AutoUpdateEnabled: true, AutoUpdateTrustRequired: true, AutoUpdateDir: dir, AutoUpdateMaxMB: 1}, &fakeLogger{}, func() config.CollectPrefs {
		calls++
		if calls == 1 {
			return config.CollectPrefs{Version: "7"}
		}
		return config.CollectPrefs{Version: "8"}
	})
	result := uc.CheckUpdateReadiness(context.Background())
	checks := result["checks"].(map[string]bool)
	if result["ready"] != false || checks["no_pending_update"] || checks["config_unchanged"] {
		t.Fatal("pending/drift accepted")
	}
	after, err := os.ReadFile(pending)
	if err != nil || string(after) != string(data) {
		t.Fatal("inspection altered pending state")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("inspection downloaded or retained another file")
	}
}

func TestSelfHealUpdateReadinessUsesOnlyExplicitProbe(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	calls := 0
	executor := NewSelfHealExecutor(log, &fakeSelfHealReporter{}, SelfHealDeps{
		UpdateReadiness: func(context.Context) map[string]any {
			calls++
			return map[string]any{"schema": UpdateReadinessSchema, "ready": false}
		},
		RuntimeSnapshot: func() map[string]any { t.Fatal("readiness should not collect unrelated snapshot"); return nil },
	})
	command := entities.SelfHealCommand{CommandID: "probe-1", Code: "inspect_runtime_config", Payload: map[string]any{"update_readiness": true}}
	status, _, result := executor.Execute(context.Background(), command)
	if status != "success" || calls != 1 || result["update_readiness"] == nil {
		t.Fatal("readiness not returned")
	}
	command.Payload["update_readiness"] = "true"
	status, _, _ = executor.Execute(context.Background(), command)
	if status != "failed" || calls != 1 {
		t.Fatal("malformed flag accepted")
	}
}

func TestUpdateReadinessCancelledDoesNotTouchStaging(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "uncreated")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	uc := NewSelfUpdate(config.Config{AutoUpdateEnabled: true, AutoUpdateDir: dir}, &fakeLogger{})
	result := uc.CheckUpdateReadiness(ctx)
	if result["ready"] != false {
		t.Fatal("cancelled probe accepted")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("cancelled probe touched staging")
	}
}

func TestSelfHealUpdateReadinessRequiresSupportedCallback(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	executor := NewSelfHealExecutor(log, &fakeSelfHealReporter{}, SelfHealDeps{})
	status, _, _ := executor.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "probe-unsupported", Code: "inspect_runtime_config", Payload: map[string]any{"update_readiness": true},
	})
	if status != "failed" {
		t.Fatal("missing readiness support accepted")
	}
}
