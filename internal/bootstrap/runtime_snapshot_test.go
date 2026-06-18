package app

import (
	"encoding/json"
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
	t.Setenv("PRIVACY_PROFILE", "sensitive")
	t.Setenv("SENSITIVE_MODE", "true")

	cfg := config.Config{
		AgentMode:            "hub",
		APIBaseURL:           "https://api.runtime.local",
		HubURL:               "https://hub.runtime.local",
		HubListenAddr:        ":9090",
		SkipBootstrap:        true,
		SelfHealPollInterval: 30 * time.Second,
		HTTPGzip:             true,
		HTTPIdempotency:      true,
		OutboxFlushBatch:     77,
		OutboxFlushInterval:  12 * time.Second,
		OutboxMaxMB:          200,
		AgentlessEnabled:     true,
		PrefsPath:            prefsPath,
		OutboxPath:           outboxPath,
		AgentlessOutboxPath:  agentlessOutboxPath,
		AgentlessOutboxMaxMB: 55,
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

	evidence, ok := snap["contextual_evidence"].(map[string]any)
	if !ok {
		t.Fatalf("contextual_evidence missing or invalid: %#v", snap["contextual_evidence"])
	}
	localAI, ok := evidence["local_ai"].(map[string]any)
	if !ok {
		t.Fatalf("local_ai missing: %#v", evidence)
	}
	if localAI["llm_required"] != false || localAI["destructive_action"] != false {
		t.Fatalf("local_ai must be deterministic and non destructive: %#v", localAI)
	}
	noiseReduction, ok := localAI["noise_reduction"].(map[string]any)
	if !ok {
		t.Fatalf("noise_reduction missing: %#v", localAI)
	}
	if noiseReduction["enabled"] != true || noiseReduction["keeps_original_evidence"] != true || noiseReduction["drops_raw_events"] != false || noiseReduction["requires_benchmark"] != true {
		t.Fatalf("noise reduction must be assistive and benchmark-gated: %#v", noiseReduction)
	}
	verdictPolicy, ok := localAI["verdict_policy"].(map[string]any)
	if !ok {
		t.Fatalf("verdict_policy missing: %#v", localAI)
	}
	if verdictPolicy["automatic_verdict"] != false || verdictPolicy["human_review_required"] != true || verdictPolicy["decision_scope"] != "triage_only" {
		t.Fatalf("local_ai must not create automatic verdicts: %#v", verdictPolicy)
	}
	if !strings.Contains(fmt.Sprint(verdictPolicy["blocked_actions"]), "execute_command") {
		t.Fatalf("verdict policy must block remote execution: %#v", verdictPolicy)
	}
	privacy, ok := evidence["privacy"].(map[string]any)
	if !ok {
		t.Fatalf("privacy missing: %#v", evidence)
	}
	if privacy["profile"] != "sensitive" || privacy["sensitive_mode"] != true {
		t.Fatalf("unexpected privacy evidence: %#v", privacy)
	}
	offline, ok := evidence["offline_first"].(map[string]any)
	if !ok {
		t.Fatalf("offline_first missing: %#v", evidence)
	}
	if offline["compression_supported"] != true || offline["http_idempotency"] != true {
		t.Fatalf("offline evidence must expose compression and idempotent replay: %#v", offline)
	}
	retention, ok := offline["retention_policy"].(map[string]any)
	if !ok {
		t.Fatalf("retention_policy missing: %#v", offline)
	}
	if retention["local_outbox_max_mb"] != 200 || retention["agentless_outbox_max_mb"] != 55 || retention["flush_batch"] != 77 {
		t.Fatalf("unexpected retention policy: %#v", retention)
	}
	replaySafety, ok := offline["replay_safety"].(map[string]any)
	if !ok {
		t.Fatalf("replay_safety missing: %#v", offline)
	}
	if replaySafety["durable_until_ack"] != true || replaySafety["ack_idempotent"] != true || replaySafety["requires_24h_validation"] != true {
		t.Fatalf("replay safety must expose durable/idempotent local contract and real validation gate: %#v", replaySafety)
	}
	if replaySafety["relay_to_hub_only"] != false || replaySafety["direct_api_from_relay"] != false {
		t.Fatalf("hub mode must not be marked as relay direct API: %#v", replaySafety)
	}
	localExport, ok := offline["local_export"].(map[string]any)
	if !ok {
		t.Fatalf("local_export missing: %#v", offline)
	}
	signature := fmt.Sprint(localExport["signature"])
	if localExport["signed"] != true || localExport["signature_algorithm"] != "sha256" || len(signature) != 64 {
		t.Fatalf("local export should include deterministic sha256 signature: %#v", localExport)
	}
	benchmark, ok := evidence["superiority_benchmark"].(map[string]any)
	if !ok {
		t.Fatalf("superiority_benchmark missing: %#v", evidence)
	}
	if benchmark["claim_allowed"] != false {
		t.Fatalf("superiority claim must remain blocked without benchmark: %#v", benchmark)
	}
	if benchmark["status"] != "pending_evidence" {
		t.Fatalf("benchmark must remain pending until objective evidence exists: %#v", benchmark)
	}
	comparisonPolicy, ok := benchmark["comparison_policy"].(map[string]any)
	if !ok {
		t.Fatalf("comparison_policy missing: %#v", benchmark)
	}
	if comparisonPolicy["declare_superiority_without_benchmark"] != false || comparisonPolicy["requires_same_scenario"] != true || comparisonPolicy["requires_raw_evidence_reference"] != true {
		t.Fatalf("comparison policy must block weak superiority claims: %#v", comparisonPolicy)
	}
	scenarios, ok := benchmark["scenarios"].([]map[string]any)
	if !ok {
		t.Fatalf("benchmark scenarios missing: %#v", benchmark)
	}
	if len(scenarios) != 4 {
		t.Fatalf("expected 4 benchmark scenarios, got %#v", scenarios)
	}
	if !strings.Contains(fmt.Sprint(benchmark["required_evidence"]), "datadog_reference") {
		t.Fatalf("benchmark must require Datadog reference evidence: %#v", benchmark)
	}
}

func TestBuildOfflineFirstEvidenceRelayKeepsHubOnlyTopology(t *testing.T) {
	t.Helper()

	offline := buildOfflineFirstEvidence(config.Config{
		HTTPIdempotency:      true,
		OutboxFlushBatch:     10,
		OutboxFlushInterval:  time.Second,
		OutboxMaxMB:          100,
		AgentlessOutboxMaxMB: 50,
	}, "relay", usecase.AgentlessSettings{FlushBatch: 5})

	replaySafety, ok := offline["replay_safety"].(map[string]any)
	if !ok {
		t.Fatalf("replay_safety missing: %#v", offline)
	}
	if replaySafety["relay_to_hub_only"] != true || replaySafety["direct_api_from_relay"] != false {
		t.Fatalf("relay replay must preserve relay -> hub -> AIceberg topology: %#v", replaySafety)
	}
}

func TestBuildContextualEvidenceMinimalProfileAvoidsRawSecrets(t *testing.T) {
	t.Setenv("PRIVACY_PROFILE", "minimal")
	t.Setenv("SENSITIVE_MODE", "true")

	evidence := buildContextualEvidenceSnapshot(config.Config{
		APIKey:   "raw-api-secret",
		HubToken: "raw-hub-secret",
	}, "direct", config.CollectPrefs{
		Logs:               false,
		Processes:          false,
		Services:           false,
		Network:            false,
		Inventory:          false,
		OSLogDiag:          false,
		NetworkPassiveMode: "socket",
	}, usecase.AgentlessSettings{Enabled: false}, false)

	privacy := evidence["privacy"].(map[string]any)
	if privacy["profile"] != "minimal" || privacy["sensitive_mode"] != true || privacy["raw_secret_logging"] != false {
		t.Fatalf("unexpected minimal privacy evidence: %#v", privacy)
	}
	minimized := fmt.Sprint(privacy["minimized_collectors"])
	for _, collector := range []string{"logs", "processes", "services", "network", "inventory", "oslog_diag"} {
		assertTextContains(t, minimized, collector)
	}
	hostEvidence := evidence["host_evidence"].(map[string]any)
	policy := hostEvidence["noc_soc_attachment_policy"].(map[string]any)
	if policy["send_raw_sensitive_payload"] != false || policy["attach_summary"] != true {
		t.Fatalf("unexpected attachment policy: %#v", policy)
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal contextual evidence: %v", err)
	}
	for _, secret := range []string{"raw-api-secret", "raw-hub-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("contextual evidence leaked raw secret %q: %s", secret, raw)
		}
	}
}

func TestBuildLocalAINoiseReductionIsAssistiveOnly(t *testing.T) {
	noise := buildLocalAINoiseReduction(config.CollectPrefs{
		Logs:      false,
		Processes: false,
		Services:  false,
		Network:   false,
	})

	if noise["enabled"] != true || noise["strategy"] != "deterministic_preclassification" {
		t.Fatalf("unexpected noise reduction strategy: %#v", noise)
	}
	for _, key := range []string{
		"keeps_original_evidence",
		"requires_benchmark",
		"human_review_for_closure",
	} {
		if noise[key] != true {
			t.Fatalf("noise reduction must keep %s=true, got %#v", key, noise)
		}
	}
	for _, key := range []string{"drops_raw_events", "automatic_suppression"} {
		if noise[key] != false {
			t.Fatalf("noise reduction must keep %s=false, got %#v", key, noise)
		}
	}
	disabledCollectors := fmt.Sprint(noise["disabled_collectors"])
	for _, collector := range []string{"logs", "processes", "services", "network"} {
		assertTextContains(t, disabledCollectors, collector)
	}
	assertTextContains(t, fmt.Sprint(noise["signals"]), "duplicate_candidate")
	assertTextContains(t, fmt.Sprint(noise["inputs"]), "redacted_logs")
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

func assertTextContains(t *testing.T, value, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected %q to contain %q", value, expected)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
