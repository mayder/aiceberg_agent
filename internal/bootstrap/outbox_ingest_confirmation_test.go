package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestOwnIngestConfigIsConfirmedBeforeConditionalPollingResumes(t *testing.T) {
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	current := store.Get()
	current.Version = "40"
	if err := store.Update(current); err != nil {
		t.Fatal(err)
	}
	var reports atomic.Int32
	server := newConfigConfirmationServer(t, &reports)
	defer server.Close()
	syncConfig := usecase.NewConfigSync(config.Config{APIBaseURL: server.URL, Agent: config.AgentCfg{Token: "owner"}}, testBootstrapLogger{}, store, nil)
	apply := func(_ string, cfg usecase.IngestConfig) {
		if _, _, err := usecase.ApplyConfigPayloadWithSecurity(testBootstrapLogger{}, store, nil, *cfg.Payload, usecase.ConfigSecurityOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	apply = confirmOwnIngestConfig("Token owner", func() string { return store.Get().Version }, syncConfig.RequestFullSync, apply)
	apply("Token owner", usecase.IngestConfig{Payload: &usecase.ConfigPayload{Version: "41"}})
	if store.Get().Version != "41" || reports.Load() != 0 {
		t.Fatal("ingest must persist before the periodic sync")
	}
	if err := syncConfig.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reports.Load() != 1 {
		t.Fatalf("applied version 41 must be confirmed, reports=%d", reports.Load())
	}
	if err := syncConfig.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reports.Load() != 1 {
		t.Fatal("unchanged config must resume conditional polling")
	}
}

func newConfigConfirmationServer(t *testing.T, reports *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token owner" {
			t.Error("owner authentication missing")
		}
		switch r.URL.Path {
		case "/v1/agent/config":
			if r.URL.Query().Get("version") == "41" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "41", "collect": map[string]any{}})
		case "/v1/agent/config-report":
			var body struct {
				Version string `json:"config_version"`
				Status  string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Version != "41" || body.Status != "applied" {
				t.Errorf("invalid report: %#v", body)
			}
			reports.Add(1)
		default:
			http.NotFound(w, r)
		}
	}))
}
