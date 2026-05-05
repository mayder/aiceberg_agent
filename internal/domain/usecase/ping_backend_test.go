package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
)

func TestPingBackendExecuteUsesLegacyPingEndpoint(t *testing.T) {
	var getCount int
	var postCount int
	var getIdentity string
	var postIdentity string
	var postIdentityPayload any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/ping" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Token legacy-token" {
			t.Fatalf("unexpected Authorization %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodGet:
			getCount++
			getIdentity = r.Header.Get("X-Agent-Identity")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": "challenge-1"})
		case http.MethodPost:
			postCount++
			postIdentity = r.Header.Get("X-Agent-Identity")
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode ack: %v", err)
			}
			if payload["challenge"] != "challenge-1" {
				t.Fatalf("unexpected challenge %#v", payload["challenge"])
			}
			postIdentityPayload = payload["agent_identity"]
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	cfg := config.Config{
		Agent:               config.AgentCfg{Token: "legacy-token"},
		APIBaseURL:          server.URL,
		AgentMode:           "direct",
		AgentClientID:       7,
		AgentID:             42,
		AgentInstallationID: "install-01",
	}
	ping := NewPingBackend(cfg, logger.New("test"))

	if err := ping.Execute(context.Background()); err != nil {
		t.Fatalf("execute ping: %v", err)
	}
	if getCount != 1 || postCount != 1 {
		t.Fatalf("expected one GET and one POST, got get=%d post=%d", getCount, postCount)
	}
	if getIdentity == "" || postIdentity == "" {
		t.Fatalf("expected identity header on GET and POST, get=%q post=%q", getIdentity, postIdentity)
	}
	identity, ok := postIdentityPayload.(map[string]any)
	if !ok {
		t.Fatalf("expected agent_identity payload, got %#v", postIdentityPayload)
	}
	if identity["schema_version"] != config.AgentIdentitySchemaVersion || identity["signature"] == "" {
		t.Fatalf("unexpected identity payload %#v", identity)
	}
}

func TestPingBackendRelayUsesHubURLForLegacyPing(t *testing.T) {
	apiCalls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		http.Error(w, "api should not be called", http.StatusInternalServerError)
	}))
	defer api.Close()

	hubCalls := 0
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hubCalls++
		if r.URL.Path != "/v1/agent/ping" {
			t.Fatalf("unexpected hub path %s", r.URL.Path)
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": "relay-challenge"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer hub.Close()

	cfg := config.Config{
		Agent:      config.AgentCfg{Token: "relay-token"},
		APIBaseURL: api.URL,
		AgentMode:  "relay",
		HubURL:     hub.URL,
	}
	ping := NewPingBackend(cfg, logger.New("test"))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ping.Execute(ctx); err != nil {
		t.Fatalf("execute relay ping: %v", err)
	}
	if apiCalls != 0 {
		t.Fatalf("relay ping must not call API directly, calls=%d", apiCalls)
	}
	if hubCalls != 2 {
		t.Fatalf("expected relay ping through hub GET+POST, calls=%d", hubCalls)
	}
}
