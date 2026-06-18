package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	agentruntime "github.com/you/aiceberg_agent/internal/domain/runtime"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestBuildSelfHealRuntimeSnapshotSanitizesSecretsAndIncludesRuntime(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	prefsPath := filepath.Join(dir, "collect_prefs.json")
	outboxPath := filepath.Join(dir, "outbox.db")
	agentlessOutboxPath := filepath.Join(dir, "agentless_outbox.db")
	agentlessTargetsPath := filepath.Join(dir, "agentless_targets.json")
	tokenPath := filepath.Join(dir, "agent.token")
	envPath := filepath.Join(dir, "agent.env")

	mustWriteFile(t, prefsPath, "{}")
	mustWriteFile(t, outboxPath, "db")
	mustWriteFile(t, agentlessOutboxPath, "db")
	mustWriteFile(t, agentlessTargetsPath, `{"jobs":[]}`)
	mustWriteFile(t, tokenPath, "token-file")
	mustWriteFile(t, envPath, strings.Join([]string{
		"AGENT_MODE=hub",
		"HUB_URL=https://hub.env.local",
		"API_BASE_URL=https://api.env.local",
		"AGENT_TOKEN=file-secret-1234",
		"AGENT_IDENTITY_SECRET=file-identity-secret-2468",
		"HUB_TOKEN=file-hub-secret-5678",
		"UNRELATED_KEY=must_not_appear",
	}, "\n"))

	t.Setenv("AGENT_ENV_FILE", envPath)
	t.Setenv("AGENT_TOKEN_PATH", tokenPath)
	t.Setenv("AGENTLESS_TARGETS_PATH", agentlessTargetsPath)
	t.Setenv("AGENT_TOKEN", "runtime-secret-abc123")
	t.Setenv("AGENT_IDENTITY_SECRET", "runtime-identity-secret-1357")
	t.Setenv("HUB_TOKEN", "runtime-hub-secret-def456")
	t.Setenv("API_KEY", "runtime-api-secret-xyz789")
	t.Setenv("SELFHEAL_POLL_INTERVAL", "45")

	cfg := config.Config{
		AgentMode:            "hub",
		APIBaseURL:           "https://api.runtime.local",
		HubURL:               "https://hub.runtime.local",
		HubListenAddr:        ":9090",
		SkipBootstrap:        true,
		SelfHealPollInterval: 30 * time.Second,
		AgentlessEnabled:     true,
		PrefsPath:            prefsPath,
		OutboxPath:           outboxPath,
		AgentlessOutboxPath:  agentlessOutboxPath,
	}
	prefs := config.CollectPrefs{
		Version:            "cfg-v42",
		Paused:             false,
		CPU:                true,
		NetworkPassiveMode: "safe",
		AgentlessEnabled:   false,
		AgentlessPollSec:   18,
	}
	settings := usecase.AgentlessSettings{
		Enabled:    false,
		PollSec:    21,
		FlushSec:   22,
		JobsLimit:  23,
		LockSec:    24,
		FlushBatch: 25,
	}

	snap := buildSelfHealRuntimeSnapshot(cfg, "hub", prefs, settings, true, nil)

	if snap["agent_mode_runtime"] != "hub" {
		t.Fatalf("unexpected agent_mode_runtime: %#v", snap["agent_mode_runtime"])
	}
	if snap["prefs_version"] != "cfg-v42" {
		t.Fatalf("unexpected prefs_version: %#v", snap["prefs_version"])
	}
	if snap["agentless_effective_enabled"] != false {
		t.Fatalf("unexpected agentless_effective_enabled: %#v", snap["agentless_effective_enabled"])
	}
	if snap["agent_pipeline_version"] != "2-compatible" {
		t.Fatalf("unexpected agent_pipeline_version: %#v", snap["agent_pipeline_version"])
	}
	scheduler, ok := snap["scheduler_snapshot"].(agentruntime.SchedulerSnapshot)
	if !ok {
		t.Fatalf("scheduler_snapshot missing or invalid: %#v", snap["scheduler_snapshot"])
	}
	if scheduler.PipelineVersion != "2-compatible" {
		t.Fatalf("unexpected scheduler pipeline version: %#v", scheduler)
	}
	if len(scheduler.Collectors) == 0 {
		t.Fatalf("expected collector specs in scheduler snapshot: %#v", scheduler.Collectors)
	}

	agentEnv, ok := snap["agent_env"].(map[string]any)
	if !ok {
		t.Fatalf("agent_env missing or invalid: %#v", snap["agent_env"])
	}
	if agentEnv["path"] != envPath {
		t.Fatalf("unexpected env path: %#v", agentEnv["path"])
	}
	if agentEnv["selfheal_poll_sec"] != 30 {
		t.Fatalf("unexpected selfheal_poll_sec: %#v", agentEnv["selfheal_poll_sec"])
	}

	runtimeValues, ok := agentEnv["runtime_values"].(map[string]any)
	if !ok {
		t.Fatalf("runtime_values missing or invalid: %#v", agentEnv["runtime_values"])
	}
	assertMaskedToken(t, runtimeValues, "AGENT_TOKEN", "runtime-secret-abc123")
	assertMaskedToken(t, runtimeValues, "AGENT_IDENTITY_SECRET", "runtime-identity-secret-1357")
	assertMaskedToken(t, runtimeValues, "HUB_TOKEN", "runtime-hub-secret-def456")
	assertMaskedToken(t, runtimeValues, "API_KEY", "runtime-api-secret-xyz789")

	fileValues, ok := agentEnv["file_values"].(map[string]any)
	if !ok {
		t.Fatalf("file_values missing or invalid: %#v", agentEnv["file_values"])
	}
	assertMaskedToken(t, fileValues, "AGENT_TOKEN", "file-secret-1234")
	assertMaskedToken(t, fileValues, "AGENT_IDENTITY_SECRET", "file-identity-secret-2468")
	assertMaskedToken(t, fileValues, "HUB_TOKEN", "file-hub-secret-5678")
	if _, exists := fileValues["UNRELATED_KEY"]; exists {
		t.Fatalf("UNRELATED_KEY should not be present in allowlist snapshot")
	}

	localState, ok := snap["local_state"].(map[string]any)
	if !ok {
		t.Fatalf("local_state missing or invalid: %#v", snap["local_state"])
	}
	targetsMeta, ok := localState["agentless_targets"].(map[string]any)
	if !ok {
		t.Fatalf("agentless_targets metadata missing: %#v", localState["agentless_targets"])
	}
	if targetsMeta["exists"] != true {
		t.Fatalf("expected agentless_targets file to exist: %#v", targetsMeta)
	}
	tokenMeta, ok := localState["agent_token_storage"].(map[string]any)
	if !ok {
		t.Fatalf("agent_token_storage metadata missing: %#v", localState["agent_token_storage"])
	}
	if tokenMeta["exists"] != true {
		t.Fatalf("expected token file to exist: %#v", tokenMeta)
	}
}

func assertMaskedToken(t *testing.T, values map[string]any, key, raw string) {
	t.Helper()

	got, ok := values[key]
	if !ok {
		t.Fatalf("key %s missing in snapshot", key)
	}
	gotStr := fmt.Sprint(got)
	if gotStr == raw {
		t.Fatalf("key %s leaked raw secret", key)
	}
	if !strings.HasSuffix(gotStr, raw[len(raw)-4:]) {
		t.Fatalf("key %s should keep suffix for traceability, got %q", key, gotStr)
	}
	if !strings.Contains(gotStr, "*") {
		t.Fatalf("key %s should be masked, got %q", key, gotStr)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
