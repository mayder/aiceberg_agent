package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesAgentModeOverridePath(t *testing.T) {
	dir := t.TempDir()
	overridePath := filepath.Join(dir, "agent_mode.override")
	if err := os.WriteFile(overridePath, []byte("relay\n"), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("AGENT_MODE", "direct")
	t.Setenv("AGENT_MODE_OVERRIDE_PATH", overridePath)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Mode() != "relay" {
		t.Fatalf("expected relay mode from override, got %q", cfg.Mode())
	}
	if cfg.AgentModeOverridePath != overridePath {
		t.Fatalf("unexpected override path: %q", cfg.AgentModeOverridePath)
	}
}

func TestLoadBlocksTLSInsecureForProductionAPI(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("API_BASE_URL", "https://api.aiceberg.com.br")
	t.Setenv("TLS_INSECURE_SKIP_VERIFY", "true")

	if _, err := Load(""); err == nil {
		t.Fatalf("expected TLS insecure blocked for production API")
	}
}

func TestLoadUsesConfigurableMetricsInterval(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("METRICS_INTERVAL", "45")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.MetricsInterval != 45*time.Second {
		t.Fatalf("unexpected metrics interval: %s", cfg.MetricsInterval)
	}
}

func TestLoadAppliesEDRSafeDefaults(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("EDR_SAFE", "true")
	t.Setenv("EDR_SAFE_PROFILE", "crowdstrike")
	t.Setenv("OSLOG_MIN_SEVERITY", "info")
	t.Setenv("OSLOG_DIAG", "true")
	t.Setenv("LOG_DISCOVERY_MAX_SOURCES", "500")
	t.Setenv("CONTEXTUAL_EVIDENCE_MAX_BYTES", "524288")
	t.Setenv("CONTAINERS_MAX_CONTAINERS", "200")
	t.Setenv("K8S_MAX_EVENTS", "1000")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.EDRSafe {
		t.Fatalf("expected EDR safe enabled")
	}
	if cfg.EDRSafeProfile != "crowdstrike" {
		t.Fatalf("unexpected profile: %q", cfg.EDRSafeProfile)
	}
	if cfg.OSLogMinSeverity != "error" {
		t.Fatalf("expected min severity error, got %q", cfg.OSLogMinSeverity)
	}
	if cfg.OSLogDiag {
		t.Fatalf("diagnostic log mode must be disabled in EDR safe mode")
	}
	if cfg.LogDiscoveryMaxCandidates > 100 || cfg.LogDiscoveryMaxEvidenceBytes > 1024 {
		t.Fatalf("expected discovery caps, got candidates=%d bytes=%d", cfg.LogDiscoveryMaxCandidates, cfg.LogDiscoveryMaxEvidenceBytes)
	}
	if cfg.ContainerMaxItems > 100 || cfg.KubernetesMaxEvents > 50 {
		t.Fatalf("expected container/k8s caps, got containers=%d k8s_events=%d", cfg.ContainerMaxItems, cfg.KubernetesMaxEvents)
	}
}

func TestLoadFallsBackWhenMetricsIntervalIsNotPositive(t *testing.T) {
	t.Setenv("AGENT_TOKEN", "token")
	t.Setenv("METRICS_INTERVAL", "0")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.MetricsInterval != 10*time.Second {
		t.Fatalf("unexpected fallback metrics interval: %s", cfg.MetricsInterval)
	}
}

func TestAgentIdentityClaimSignsDeclaredIdentityWithoutRawToken(t *testing.T) {
	cfg := Config{
		Agent:               AgentCfg{Token: "agent-token-secret"},
		AgentClientID:       7,
		AgentID:             42,
		AgentInstallationID: "install-01",
	}

	claim := cfg.AgentIdentityClaim("host-01")
	if claim["schema_version"] != AgentIdentitySchemaVersion {
		t.Fatalf("unexpected schema %#v", claim["schema_version"])
	}
	if claim["cliente_id"] != 7 || claim["agente_id"] != 42 {
		t.Fatalf("unexpected identity fields %#v", claim)
	}
	if claim["signature"] == "" {
		t.Fatalf("signature missing: %#v", claim)
	}
	if _, ok := claim["token"]; ok {
		t.Fatalf("claim must not expose token: %#v", claim)
	}

	if signAgentIdentity(claim, "agent-token-secret") != claim["signature"] {
		t.Fatalf("signature does not match canonical payload")
	}
}

func TestAgentIdentityHeaderEncodesClaimAsBase64JSON(t *testing.T) {
	cfg := Config{
		Agent:         AgentCfg{Token: "agent-token-secret"},
		AgentClientID: 7,
		AgentID:       42,
	}

	header := cfg.AgentIdentityHeader("")
	raw, err := base64.RawURLEncoding.DecodeString(header)
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["schema_version"] != AgentIdentitySchemaVersion {
		t.Fatalf("unexpected payload %#v", payload)
	}
	if payload["signature"] == "" {
		t.Fatalf("signature missing: %#v", payload)
	}
}
