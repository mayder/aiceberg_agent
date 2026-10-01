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
	var identityHeader string
	var version string
	var reports int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/agent/config-report" {
			reports++
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/v1/agent/config" {
			http.NotFound(w, r)
			return
		}
		identityHeader = r.Header.Get("X-Agent-Identity")
		version = r.URL.Query().Get("version")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL:          srv.URL,
		Agent:               config.AgentCfg{Token: "t"},
		AgentClientID:       7,
		AgentID:             42,
		AgentInstallationID: "install-01",
	}
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	current := store.Get()
	current.Version = "12"
	if err := store.Update(current); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}
	log := &fakeLogger{}
	uc := NewConfigSync(cfg, log, store, nil)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if identityHeader == "" {
		t.Fatalf("expected identity header on config sync")
	}
	if version != "12" {
		t.Fatalf("expected persisted version 12, got %q", version)
	}
	if reports != 0 {
		t.Fatalf("204 must not emit config-report, got %d", reports)
	}
	if store.Get().Version != "12" {
		t.Fatalf("204 changed local preferences")
	}
}

func TestConfigSync_EmptyPrefsOmitsVersion(t *testing.T) {
	var queryVersion string
	var hasVersion bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values, present := r.URL.Query()["version"]
		hasVersion = present
		if len(values) > 0 {
			queryVersion = values[0]
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	uc := NewConfigSync(config.Config{APIBaseURL: srv.URL, Agent: config.AgentCfg{Token: "t"}}, &fakeLogger{}, prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json")), nil)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if hasVersion || queryVersion != "" {
		t.Fatalf("empty prefs must omit version, got present=%v value=%q", hasVersion, queryVersion)
	}
}

func TestConfigSync_VersionedPrefsSendVersionOnHTTP200(t *testing.T) {
	var gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/config":
			gotVersion = r.URL.Query().Get("version")
			_ = json.NewEncoder(w).Encode(map[string]any{"version": "22", "collect": map[string]any{}})
		case "/v1/agent/config-report":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	current := store.Get()
	current.Version = "21"
	if err := store.Update(current); err != nil {
		t.Fatalf("seed prefs: %v", err)
	}
	uc := NewConfigSync(config.Config{APIBaseURL: srv.URL, Agent: config.AgentCfg{Token: "t"}}, &fakeLogger{}, store, nil)
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if gotVersion != "21" {
		t.Fatalf("expected request version 21, got %q", gotVersion)
	}
	if store.Get().Version != "22" {
		t.Fatalf("expected HTTP 200 payload to remain applicable")
	}
}

func TestConfigSync_AppliesPayload(t *testing.T) {
	const checksum = "2689367b205c16ce32ca6f3d2f0a21f9923f5f0f68e6f4f7638f353cec3588f3"
	var configReport struct {
		Status        string `json:"status"`
		ConfigVersion string `json:"config_version"`
		ConfigHash    string `json:"config_hash"`
		Message       string `json:"message"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/config":
		case "/v1/agent/config-report":
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST config-report, got %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&configReport); err != nil {
				t.Fatalf("decode config report: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		default:
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
	if configReport.Status != "applied" {
		t.Fatalf("expected applied config report, got %#v", configReport)
	}
	if configReport.ConfigVersion != "e2e-1" {
		t.Fatalf("expected config version e2e-1, got %q", configReport.ConfigVersion)
	}
	if configReport.ConfigHash == "" {
		t.Fatalf("expected config hash in report")
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

func TestConfigSyncBackoffSkipsTransientFailure(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "temporary", http.StatusBadGateway)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		Agent:      config.AgentCfg{Token: "t"},
	}
	store := prefs.NewStore(filepath.Join(t.TempDir(), "prefs.json"))
	log := &fakeLogger{}
	uc := NewConfigSync(cfg, log, store, nil)

	if err := uc.Execute(context.Background()); err == nil {
		t.Fatalf("expected first transient failure")
	}
	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("expected second execution to be skipped by backoff, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected second execution not to call API, got %d calls", calls)
	}
	if len(log.err) != 0 {
		t.Fatalf("transient config sync failure must not be logged as ERROR, got %#v", log.err)
	}
}
