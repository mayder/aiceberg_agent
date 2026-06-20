package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentControlClientFetchSelfHealCommandsDirect(t *testing.T) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/selfheal-commands" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Token test-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("X-Agent-Identity"); got == "" {
			t.Fatalf("expected identity header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"commands": []map[string]any{
				{
					"command_id": "cmd-1",
					"code":       "reload_configuration",
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL:    srv.URL,
		AgentMode:     "direct",
		Agent:         config.AgentCfg{Token: "test-token"},
		AgentClientID: 7,
		AgentID:       42,
	}
	client := NewAgentControlClient(cfg)

	commands, err := client.FetchSelfHealCommands(context.Background())
	if err != nil {
		t.Fatalf("fetch commands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].CommandID != "cmd-1" || commands[0].Code != "reload_configuration" {
		t.Fatalf("unexpected command payload: %#v", commands[0])
	}
}

func TestAgentControlClientReportsSendIdentityHeader(t *testing.T) {
	t.Helper()

	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("X-Agent-Identity")
		if got := r.Header.Get("Authorization"); got != "Token test-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL:    srv.URL,
		AgentMode:     "direct",
		Agent:         config.AgentCfg{Token: "test-token"},
		AgentClientID: 7,
		AgentID:       42,
	}
	client := NewAgentControlClient(cfg)

	if err := client.ReportSelfHeal(context.Background(), entities.SelfHealReport{CommandID: "cmd-1", Status: "success"}); err != nil {
		t.Fatalf("report selfheal: %v", err)
	}
	if err := client.ReportWorkerErrors(context.Background(), []entities.WorkerErrorEvent{{ErrorType: "test_error", Severity: "warning", RecoveryStatus: "open"}}); err != nil {
		t.Fatalf("report worker error: %v", err)
	}
	if seen["/v1/agent/selfheal-report"] == "" {
		t.Fatalf("expected identity header on selfheal report")
	}
	if seen["/v1/agent/error-report"] == "" {
		t.Fatalf("expected identity header on worker error report")
	}
}

func TestAgentControlClientFetchSelfHealCommandsRelayUsesHub(t *testing.T) {
	t.Helper()

	apiCalls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		t.Fatalf("api server should not be called in relay fetch, path=%s", r.URL.Path)
	}))
	defer apiSrv.Close()

	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/selfheal-commands" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"commands": []map[string]any{
				{"command_id": "cmd-hub", "code": "inspect_runtime_config"},
			},
		})
	}))
	defer hubSrv.Close()

	cfg := config.Config{
		APIBaseURL: apiSrv.URL,
		HubURL:     hubSrv.URL,
		AgentMode:  "relay",
		Agent:      config.AgentCfg{Token: "test-token"},
	}
	client := NewAgentControlClient(cfg)

	commands, err := client.FetchSelfHealCommands(context.Background())
	if err != nil {
		t.Fatalf("fetch commands: %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(commands))
	}
	if commands[0].CommandID != "cmd-hub" {
		t.Fatalf("unexpected command id: %q", commands[0].CommandID)
	}
	if apiCalls != 0 {
		t.Fatalf("api should not be called in relay fetch, calls=%d", apiCalls)
	}
}

func TestAgentControlClientReportSelfHealRelayDoesNotFallbackToAPI(t *testing.T) {
	t.Helper()

	hubCalls := 0
	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hubCalls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer hubSrv.Close()

	apiCalls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		t.Fatalf("relay report must not call API directly, path=%s", r.URL.Path)
	}))
	defer apiSrv.Close()

	cfg := config.Config{
		APIBaseURL: apiSrv.URL,
		HubURL:     hubSrv.URL,
		AgentMode:  "relay",
		Agent:      config.AgentCfg{Token: "test-token"},
	}
	client := NewAgentControlClient(cfg)

	err := client.ReportSelfHeal(context.Background(), entities.SelfHealReport{
		CommandID: "cmd-22",
		Status:    "success",
	})
	if err == nil {
		t.Fatalf("expected relay hub error")
	}
	if hubCalls == 0 {
		t.Fatalf("expected at least one call to hub")
	}
	if apiCalls != 0 {
		t.Fatalf("api should not be called in relay report, calls=%d", apiCalls)
	}
}

func TestAgentControlClientReportSelfHealInvalidPayload(t *testing.T) {
	t.Helper()
	client := NewAgentControlClient(config.Config{})
	err := client.ReportSelfHeal(context.Background(), entities.SelfHealReport{
		CommandID: "",
		Status:    "success",
	})
	if err == nil {
		t.Fatalf("expected invalid report error")
	}
	if !strings.Contains(err.Error(), "invalid selfheal report") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentControlClientReportWorkerErrors(t *testing.T) {
	t.Helper()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/agent/error-report" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string][]entities.WorkerErrorEvent
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if len(payload["errors"]) != 1 {
			t.Fatalf("expected one error event, got %d", len(payload["errors"]))
		}
		if payload["errors"][0].ErrorType != "collect_failed" {
			t.Fatalf("unexpected error payload: %#v", payload["errors"][0])
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	cfg := config.Config{
		APIBaseURL: srv.URL,
		AgentMode:  "direct",
		Agent:      config.AgentCfg{Token: "test-token"},
	}
	client := NewAgentControlClient(cfg)
	err := client.ReportWorkerErrors(context.Background(), []entities.WorkerErrorEvent{
		{ErrorType: "collect_failed", Severity: "error"},
	})
	if err != nil {
		t.Fatalf("report worker errors: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one request, got %d", calls)
	}
}
