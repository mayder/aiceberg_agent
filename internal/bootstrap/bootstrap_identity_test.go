package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestBootstrapSendsAgentIdentityHeaderAndPayload(t *testing.T) {
	t.Setenv("AGENT_STATE_PATH", filepath.Join(t.TempDir(), "bootstrap.ok"))
	t.Setenv("AGENT_TOKEN_PATH", filepath.Join(t.TempDir(), "agent.token"))

	var identityHeader string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/bootstrap" {
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Token bootstrap-token" {
			t.Fatalf("unexpected auth header %q", r.Header.Get("Authorization"))
		}
		identityHeader = r.Header.Get("X-Agent-Identity")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode bootstrap payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := config.Config{
		Agent:               config.AgentCfg{Token: "bootstrap-token"},
		APIBaseURL:          server.URL,
		AgentClientID:       7,
		AgentID:             42,
		AgentInstallationID: "install-01",
	}

	if err := bootstrap(context.Background(), cfg, testBootstrapLogger{}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if identityHeader == "" {
		t.Fatalf("expected identity header")
	}
	identity, ok := payload["agent_identity"].(map[string]any)
	if !ok {
		t.Fatalf("expected agent_identity payload, got %#v", payload["agent_identity"])
	}
	if identity["schema_version"] != config.AgentIdentitySchemaVersion || identity["signature"] == "" {
		t.Fatalf("unexpected identity payload %#v", identity)
	}
	if _, ok := identity["token"]; ok {
		t.Fatalf("identity payload must not expose raw token: %#v", identity)
	}
}

func TestInitialBootstrapFailureDegradesWithoutFatal(t *testing.T) {
	t.Setenv("AGENT_STATE_PATH", filepath.Join(t.TempDir(), "bootstrap.ok"))
	t.Setenv("AGENT_TOKEN_PATH", filepath.Join(t.TempDir(), "agent.token"))

	log := &recordBootstrapLogger{}
	runInitialBootstrap(context.Background(), config.Config{
		Agent:      config.AgentCfg{Token: "bootstrap-token"},
		APIBaseURL: "http://127.0.0.1:1",
	}, log)

	if log.fatalCalled {
		t.Fatalf("bootstrap failure must not terminate the agent")
	}
	if !strings.Contains(log.errorMessage, "bootstrap degraded") {
		t.Fatalf("expected degraded bootstrap log, got %q", log.errorMessage)
	}
}

func TestInitialBootstrapRetriesUntilAPIRecovers(t *testing.T) {
	t.Setenv("AGENT_STATE_PATH", filepath.Join(t.TempDir(), "bootstrap.ok"))
	t.Setenv("AGENT_TOKEN_PATH", filepath.Join(t.TempDir(), "agent.token"))

	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/bootstrap" {
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
		if attempts.Add(1) == 1 {
			http.Error(w, "temporary outage", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	log := &recordBootstrapLogger{}

	if runInitialBootstrap(ctx, config.Config{
		Agent:      config.AgentCfg{Token: "bootstrap-token"},
		APIBaseURL: srv.URL,
	}, log) {
		t.Fatalf("first bootstrap attempt should degrade")
	}
	retryInitialBootstrap(ctx, config.Config{
		Agent:      config.AgentCfg{Token: "bootstrap-token"},
		APIBaseURL: srv.URL,
	}, log, 10*time.Millisecond)

	if attempts.Load() < 2 {
		t.Fatalf("expected retry after temporary outage, got %d attempts", attempts.Load())
	}
	if log.fatalCalled {
		t.Fatalf("bootstrap retry must not terminate the agent")
	}
}

type testBootstrapLogger struct{}

func (testBootstrapLogger) Info(string)          {}
func (testBootstrapLogger) Error(string)         {}
func (testBootstrapLogger) Fatal(string, ...any) {}
func (testBootstrapLogger) Sync()                {}

type recordBootstrapLogger struct {
	errorMessage string
	fatalCalled  bool
}

func (l *recordBootstrapLogger) Info(string) {}
func (l *recordBootstrapLogger) Error(msg string) {
	l.errorMessage = msg
}
func (l *recordBootstrapLogger) Fatal(string, ...any) {
	l.fatalCalled = true
}
func (l *recordBootstrapLogger) Sync() {}
