package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
