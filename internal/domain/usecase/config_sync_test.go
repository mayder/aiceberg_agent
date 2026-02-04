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
	cmd := make(chan string, 1)
	uc := NewConfigSync(cfg, log, store, cmd)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if store.Get().Version != "e2e-1" {
		t.Fatalf("expected version e2e-1, got %q", store.Get().Version)
	}
	select {
	case got := <-cmd:
		if got != "health" {
			t.Fatalf("expected command health, got %q", got)
		}
	default:
		t.Fatalf("expected command in channel")
	}
}
