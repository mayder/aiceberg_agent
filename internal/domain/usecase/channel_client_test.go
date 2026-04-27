package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/version"
	"github.com/you/aiceberg_agent/internal/domain/channel"
)

func TestAgentChannelClientOpenSendsDirectPresence(t *testing.T) {
	var gotAuth string
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != channelRoute {
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "agent-token"},
		APIBaseURL: server.URL,
		AgentMode:  "direct",
	}, testLogger{})

	if _, err := client.open(context.Background(), "session-1", channel.ModeDirect); err != nil {
		t.Fatalf("open channel: %v", err)
	}

	if gotAuth != "Token agent-token" {
		t.Fatalf("unexpected auth header %q", gotAuth)
	}
	if got["action"] != "open" || got["session_id"] != "session-1" || got["mode"] != "direct" {
		t.Fatalf("unexpected open payload %#v", got)
	}
	if got["version"] != version.Version {
		t.Fatalf("expected version %q, got %#v", version.Version, got["version"])
	}
	caps, ok := got["capabilities"].([]any)
	if !ok || len(caps) == 0 {
		t.Fatalf("expected capabilities list, got %#v", got["capabilities"])
	}
}

func TestAgentChannelClientHeartbeatSendsLatencyAndCapabilities(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "agent-token"},
		APIBaseURL: server.URL,
		AgentMode:  "direct",
	}, testLogger{})

	if _, err := client.heartbeat(context.Background(), "session-2", channel.ModeDirect, 123); err != nil {
		t.Fatalf("heartbeat channel: %v", err)
	}

	if got["action"] != "heartbeat" || got["session_id"] != "session-2" || got["mode"] != "direct" {
		t.Fatalf("unexpected heartbeat payload %#v", got)
	}
	if got["latency_ms"].(float64) != 123 {
		t.Fatalf("expected latency_ms 123, got %#v", got["latency_ms"])
	}
	if got["version"] != version.Version {
		t.Fatalf("expected version %q, got %#v", version.Version, got["version"])
	}
}

func TestAgentChannelClientRunDirectKeepsHeartbeat(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests <- got["action"].(string)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewAgentChannelClient(config.Config{
		Agent:                    config.AgentCfg{Token: "agent-token"},
		APIBaseURL:               server.URL,
		AgentMode:                "direct",
		ChannelHeartbeatInterval: 10 * time.Millisecond,
	}, testLogger{})
	client.newSessionID = func(string) string { return "session-loop" }

	go client.RunDirect(ctx)

	if action := waitAction(t, requests); action != "open" {
		t.Fatalf("expected open, got %s", action)
	}
	if action := waitAction(t, requests); action != "heartbeat" {
		t.Fatalf("expected heartbeat, got %s", action)
	}
	cancel()
}

func TestAgentChannelClientRunHubKeepsHeartbeat(t *testing.T) {
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got map[string]any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests <- got
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := NewAgentChannelClient(config.Config{
		Agent:                    config.AgentCfg{Token: "hub-token"},
		APIBaseURL:               server.URL,
		AgentMode:                "hub",
		ChannelHeartbeatInterval: 10 * time.Millisecond,
	}, testLogger{})
	client.newSessionID = func(string) string { return "hub-session-loop" }

	go client.RunHub(ctx)

	open := waitRequest(t, requests)
	if open["action"] != "open" || open["mode"] != "hub" {
		t.Fatalf("unexpected hub open payload %#v", open)
	}
	caps, ok := open["capabilities"].([]any)
	if !ok || !containsAny(caps, "channel.hub") || !containsAny(caps, "relay.command.receive") {
		t.Fatalf("expected hub capabilities, got %#v", open["capabilities"])
	}
	heartbeat := waitRequest(t, requests)
	if heartbeat["action"] != "heartbeat" || heartbeat["mode"] != "hub" {
		t.Fatalf("unexpected hub heartbeat payload %#v", heartbeat)
	}
	cancel()
}

func TestAgentChannelClientReceivesHubAndRelayCommands(t *testing.T) {
	commands := make(chan channel.Envelope, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commands": []channel.Envelope{
				{
					ContractID:    channel.ContractID,
					SchemaVersion: channel.SchemaVersion,
					MessageID:     "msg-hub",
					Type:          channel.TypeCommand,
					TimestampUTC:  time.Now().UTC().Format(time.RFC3339Nano),
					AgentID:       "hub-agent",
					Mode:          channel.ModeHub,
					CommandID:     "cmd-hub",
				},
				{
					ContractID:    channel.ContractID,
					SchemaVersion: channel.SchemaVersion,
					MessageID:     "msg-relay",
					Type:          channel.TypeCommand,
					TimestampUTC:  time.Now().UTC().Format(time.RFC3339Nano),
					AgentID:       "relay-agent",
					HubAgentID:    "hub-agent",
					Mode:          channel.ModeRelay,
					CommandID:     "cmd-relay",
				},
			},
		})
	}))
	defer server.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "hub-token"},
		APIBaseURL: server.URL,
		AgentMode:  "hub",
	}, testLogger{})
	client.handleCommand = func(_ context.Context, command channel.Envelope) error {
		commands <- command
		return nil
	}

	if _, err := client.heartbeat(context.Background(), "hub-session", channel.ModeHub, 7); err != nil {
		t.Fatalf("heartbeat channel: %v", err)
	}

	first := waitCommand(t, commands)
	second := waitCommand(t, commands)
	if first.CommandID != "cmd-hub" || second.CommandID != "cmd-relay" {
		t.Fatalf("unexpected commands %s %s", first.CommandID, second.CommandID)
	}
}

func TestAgentChannelClientRejectsInvalidServerCommands(t *testing.T) {
	commands := make(chan channel.Envelope, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"commands": []channel.Envelope{
				{
					ContractID:    channel.ContractID,
					SchemaVersion: channel.SchemaVersion,
					MessageID:     "msg-invalid",
					Type:          channel.TypeCommand,
					TimestampUTC:  time.Now().UTC().Format(time.RFC3339Nano),
					AgentID:       "agent-1",
					Mode:          channel.ModeDirect,
				},
			},
		})
	}))
	defer server.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "agent-token"},
		APIBaseURL: server.URL,
		AgentMode:  "direct",
	}, testLogger{})
	client.handleCommand = func(_ context.Context, command channel.Envelope) error {
		commands <- command
		return nil
	}

	if _, err := client.heartbeat(context.Background(), "session-invalid", channel.ModeDirect, 3); err != nil {
		t.Fatalf("heartbeat channel: %v", err)
	}

	select {
	case command := <-commands:
		t.Fatalf("invalid command must not reach handler: %#v", command)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestAgentChannelClientRunDirectSkipsRelayMode(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "agent-token"},
		APIBaseURL: server.URL,
		AgentMode:  "relay",
	}, testLogger{})
	client.RunDirect(context.Background())

	if called {
		t.Fatalf("relay mode must not connect direct channel to AIceberg")
	}
}

func TestAgentChannelClientRelayUsesHubURLOnly(t *testing.T) {
	apiCalled := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
	}))
	defer api.Close()

	var got map[string]any
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != channelRoute {
			t.Fatalf("unexpected route %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer hub.Close()

	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "relay-token"},
		APIBaseURL: api.URL,
		AgentMode:  "relay",
		HubURL:     hub.URL,
	}, testLogger{})

	if _, err := client.open(context.Background(), "relay-session", channel.ModeRelay); err != nil {
		t.Fatalf("open relay channel: %v", err)
	}

	if apiCalled {
		t.Fatalf("relay channel must not call AIceberg directly")
	}
	if got["mode"] != "relay" || got["session_id"] != "relay-session" {
		t.Fatalf("unexpected relay payload %#v", got)
	}
	caps, ok := got["capabilities"].([]any)
	if !ok || !containsAny(caps, "channel.relay") {
		t.Fatalf("expected relay capabilities, got %#v", got["capabilities"])
	}
}

func TestAgentChannelClientSnapshotTracksHeartbeatAndFallback(t *testing.T) {
	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "agent-token"},
		APIBaseURL: "http://127.0.0.1:1",
		AgentMode:  "direct",
	}, testLogger{})

	initial := client.Snapshot()
	if !initial.Enabled || !initial.FallbackActive || initial.Mode != channel.ModeDirect {
		t.Fatalf("unexpected initial snapshot %#v", initial)
	}

	client.recordConnected(channel.ModeDirect, "session-1", 42)
	connected := client.Snapshot()
	if !connected.Connected || connected.FallbackActive || connected.LastLatencyMs != 42 || connected.LastHeartbeatUTC == "" {
		t.Fatalf("unexpected connected snapshot %#v", connected)
	}

	client.recordError(channel.ModeDirect, "session-1", context.DeadlineExceeded)
	fallback := client.Snapshot()
	if fallback.Connected || !fallback.FallbackActive || fallback.ReconnectRetries != 1 || fallback.LastError == "" {
		t.Fatalf("unexpected fallback snapshot %#v", fallback)
	}
}

func TestAgentChannelClientSnapshotRelayTopology(t *testing.T) {
	client := NewAgentChannelClient(config.Config{
		Agent:      config.AgentCfg{Token: "relay-token"},
		APIBaseURL: "https://api.example.test",
		AgentMode:  "relay",
		HubURL:     "https://hub.example.test",
	}, testLogger{})

	snapshot := client.Snapshot()
	if !snapshot.RelayUsesHubURL || !snapshot.HubURLConfigured || snapshot.ConnectsToAiceberg || snapshot.RelayConnectsAiceberg {
		t.Fatalf("unexpected relay snapshot %#v", snapshot)
	}
}

func waitAction(t *testing.T, requests <-chan string) string {
	t.Helper()
	select {
	case action := <-requests:
		return action
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel request")
		return ""
	}
}

func waitRequest(t *testing.T, requests <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel request")
		return nil
	}
}

func waitCommand(t *testing.T, commands <-chan channel.Envelope) channel.Envelope {
	t.Helper()
	select {
	case command := <-commands:
		return command
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel command")
		return channel.Envelope{}
	}
}

func containsAny(values []any, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

type testLogger struct{}

func (testLogger) Info(string)          {}
func (testLogger) Error(string)         {}
func (testLogger) Fatal(string, ...any) {}
func (testLogger) Sync()                {}
