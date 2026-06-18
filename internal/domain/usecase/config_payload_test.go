package usecase

import (
	"path/filepath"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

func TestApplyConfigPayload_QueuesSelfUpdateOnSameVersion(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		Update: &UpdatePayload{
			Version: "7.0.6",
			URL:     "https://example.org/agent.bin",
		},
	}

	version, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if version != "cfg-v1" {
		t.Fatalf("expected version cfg-v1, got %q", version)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update" {
			t.Fatalf("expected self_update, got %q", cmd.Name)
		}
		if cmd.Update == nil || cmd.Update.Version != "7.0.6" {
			t.Fatalf("expected update payload version 7.0.6")
		}
	default:
		t.Fatalf("expected self_update command in channel")
	}
}

func TestApplyConfigPayload_QueuesSelfUpdatePolicy(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	enabled := true
	command := "echo update"
	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		AutoUpdate: &AutoUpdatePayload{
			Enabled: &enabled,
			Command: &command,
		},
	}

	_, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update_policy" {
			t.Fatalf("expected self_update_policy, got %q", cmd.Name)
		}
		if cmd.AutoUpdate == nil || cmd.AutoUpdate.Enabled == nil || !*cmd.AutoUpdate.Enabled {
			t.Fatalf("expected auto update enabled override")
		}
	default:
		t.Fatalf("expected self_update_policy command in channel")
	}
}

func TestApplyConfigPayload_QueuesSelfUpdatePolicyWhenPayloadHasNullFields(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	payload := ConfigPayload{
		Version:    "cfg-v1",
		Collect:    config.CollectPrefs{CPU: true},
		AutoUpdate: &AutoUpdatePayload{},
	}

	_, applied, err := ApplyConfigPayload(log, store, cmds, payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !applied {
		t.Fatalf("expected applied=true")
	}

	select {
	case cmd := <-cmds:
		if cmd.Name != "self_update_policy" {
			t.Fatalf("expected self_update_policy, got %q", cmd.Name)
		}
		if cmd.AutoUpdate == nil {
			t.Fatalf("expected auto update payload")
		}
	default:
		t.Fatalf("expected self_update_policy command in channel")
	}
}

func TestApplyConfigPayload_QueuesPolicyBeforeSelfUpdate(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 2)

	base := config.CollectPrefs{Version: "cfg-v1", CPU: true}
	if err := store.Update(base); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}

	enabled := true
	payload := ConfigPayload{
		Version: "cfg-v1",
		Collect: config.CollectPrefs{CPU: true},
		Update: &UpdatePayload{
			Version: "7.0.99",
			URL:     "https://example.org/agent.bin",
		},
		AutoUpdate: &AutoUpdatePayload{
			Enabled: &enabled,
		},
	}

	if _, _, err := ApplyConfigPayload(log, store, cmds, payload); err != nil {
		t.Fatalf("apply payload: %v", err)
	}

	first := <-cmds
	second := <-cmds
	if first.Name != "self_update_policy" {
		t.Fatalf("expected first command self_update_policy, got %q", first.Name)
	}
	if second.Name != "self_update" {
		t.Fatalf("expected second command self_update, got %q", second.Name)
	}
}

func TestApplyConfigPayloadWithSecurityRequiresSignatureForSensitivePayload(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-sec",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "99.0.0", URL: "https://example.org/agent.bin"},
	}

	_, _, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret:   "secret",
		SignatureRequired: true,
	})
	if err == nil {
		t.Fatalf("expected missing signature error")
	}
}

func TestApplyConfigPayloadWithSecurityAcceptsSignedSensitivePayload(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-sec",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "99.0.0", URL: "https://example.org/agent.bin"},
	}
	payload.Signature = PayloadSignature{Algorithm: "hmac-sha256"}
	sig, err := SignConfigPayload(payload, "secret")
	if err != nil {
		t.Fatal(err)
	}
	payload.Signature.Value = sig

	_, applied, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret:   "secret",
		SignatureRequired: true,
	})
	if err != nil {
		t.Fatalf("expected signed payload accepted, got %v", err)
	}
	if !applied {
		t.Fatalf("expected payload applied")
	}
}

func TestApplyConfigPayloadBlocksUnsignedTokenRotation(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version:       "cfg-token",
		Collect:       config.CollectPrefs{CPU: true},
		TokenRotation: &TokenRotationPayload{NewToken: "new-token"},
	}

	_, _, err := ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{
		SignatureSecret: "secret",
	})
	if err == nil {
		t.Fatalf("expected unsigned sensitive payload blocked")
	}
}

func TestApplyConfigPayloadQueuesTokenRotationWhenSigned(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version:       "cfg-token",
		Collect:       config.CollectPrefs{CPU: true},
		TokenRotation: &TokenRotationPayload{NewToken: "new-token", Reason: "rotation"},
	}
	payload.Signature = PayloadSignature{Algorithm: "hmac-sha256"}
	sig, err := SignConfigPayload(payload, "secret")
	if err != nil {
		t.Fatal(err)
	}
	payload.Signature.Value = sig

	_, _, err = ApplyConfigPayloadWithSecurity(log, store, cmds, payload, ConfigSecurityOptions{SignatureSecret: "secret"})
	if err != nil {
		t.Fatalf("expected signed token rotation accepted, got %v", err)
	}
	cmd := <-cmds
	if cmd.Name != "rotate_agent_token" || cmd.TokenRotation == nil || cmd.TokenRotation.NewToken != "new-token" {
		t.Fatalf("unexpected token rotation command: %#v", cmd)
	}
}

func TestApplyConfigPayloadBlocksDowngradeWithoutForce(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmds := make(chan ControlCommand, 1)

	payload := ConfigPayload{
		Version: "cfg-downgrade",
		Collect: config.CollectPrefs{CPU: true},
		Update:  &UpdatePayload{Version: "0.0.0", URL: "https://example.org/agent.bin"},
	}

	_, _, err := ApplyConfigPayload(log, store, cmds, payload)
	if err == nil {
		t.Fatalf("expected downgrade blocked")
	}
}
