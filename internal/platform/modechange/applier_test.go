package modechange

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTargetModeFromPayloadNormalizesDireto(t *testing.T) {
	mode, err := targetModeFromPayload(map[string]any{"target_mode": "direto"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if mode != "direct" {
		t.Fatalf("expected direct, got %q", mode)
	}
}

func TestWriteOverrideValuePersistsMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "agent_mode.override")
	if err := writeOverrideValue(path, "relay"); err != nil {
		t.Fatalf("write override: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read override: %v", err)
	}
	if string(raw) != "relay\n" {
		t.Fatalf("expected relay override, got %q", string(raw))
	}
}
