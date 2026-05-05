package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

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

type testBootstrapLogger struct{}

func (testBootstrapLogger) Info(string)          {}
func (testBootstrapLogger) Error(string)         {}
func (testBootstrapLogger) Fatal(string, ...any) {}
func (testBootstrapLogger) Sync()                {}
