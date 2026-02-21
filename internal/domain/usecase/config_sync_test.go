package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

func TestConfigSync_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/config" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		Agent:      config.AgentCfg{Token: "t"},
	}
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	uc := NewConfigSync(cfg, log, store, nil)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestConfigSync_AppliesPayload(t *testing.T) {
	const checksum = "2689367b205c16ce32ca6f3d2f0a21f9923f5f0f68e6f4f7638f353cec3588f3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/config" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"version": "e2e-1",
			"collect": map[string]any{},
			"collect_now": []string{
				"health",
			},
			"update": map[string]any{
				"version": "7.0.6",
				"url":     "https://example.org/aiceberg-agent-linux-amd64.tar.gz",
				"sha256":  checksum,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		Agent:      config.AgentCfg{Token: "t"},
	}
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	cmd := make(chan ControlCommand, 2)
	uc := NewConfigSync(cfg, log, store, cmd)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if store.Get().Version != "e2e-1" {
		t.Fatalf("expected version e2e-1, got %q", store.Get().Version)
	}
	select {
	case got := <-cmd:
		if got.Name != "health" {
			t.Fatalf("expected command health, got %q", got.Name)
		}
	default:
		t.Fatalf("expected command in channel")
	}
	select {
	case got := <-cmd:
		if got.Name != "self_update" {
			t.Fatalf("expected command self_update, got %q", got.Name)
		}
		if got.Update == nil {
			t.Fatalf("expected update payload")
		}
		if got.Update.Version != "7.0.6" {
			t.Fatalf("expected update version 7.0.6, got %q", got.Update.Version)
		}
		if got.Update.SHA256 != checksum {
			t.Fatalf("expected checksum %q, got %q", checksum, got.Update.SHA256)
		}
	default:
		t.Fatalf("expected self_update command in channel")
	}
}
