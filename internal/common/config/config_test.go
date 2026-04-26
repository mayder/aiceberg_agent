package config

import (
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
