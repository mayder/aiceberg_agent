package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestHubChannelForwardsRelayPresenceToAiceberg(t *testing.T) {
	var upstreamAuth string
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/channel" {
			t.Fatalf("unexpected upstream route %s", r.URL.Path)
		}
		upstreamAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	handler := NewHandler(config.Config{
		APIBaseURL: upstream.URL,
	}, nil, testHubLogger{}, nil)

	body := `{"action":"open","session_id":"relay-session","mode":"relay","hostname":"relay-1","version":"0.7.32"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/channel", strings.NewReader(body))
	req.Header.Set("Authorization", "Token relay-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Token relay-token" {
		t.Fatalf("expected relay auth forwarded, got %q", upstreamAuth)
	}
	if upstreamPayload["mode"] != "relay" || upstreamPayload["session_id"] != "relay-session" {
		t.Fatalf("unexpected upstream payload %#v", upstreamPayload)
	}
}

func TestHubChannelForwardsRelayCommandEventToAiceberg(t *testing.T) {
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/channel" {
			t.Fatalf("unexpected upstream route %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","duplicate":false}`))
	}))
	defer upstream.Close()

	handler := NewHandler(config.Config{
		APIBaseURL: upstream.URL,
	}, nil, testHubLogger{}, nil)

	body := `{"action":"event","session_id":"relay-session","mode":"relay","message_id":"msg-relay-ack","type":"ack","timestamp_utc":"2026-04-26T12:00:00Z","command_id":"cmd-relay","payload":{"status":"accepted"}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/channel", strings.NewReader(body))
	req.Header.Set("Authorization", "Token relay-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamPayload["action"] != "event" || upstreamPayload["mode"] != "relay" || upstreamPayload["command_id"] != "cmd-relay" {
		t.Fatalf("unexpected upstream event payload %#v", upstreamPayload)
	}
}

func TestHubChannelRejectsNonRelayMode(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
	}))
	defer upstream.Close()

	handler := NewHandler(config.Config{
		APIBaseURL: upstream.URL,
	}, nil, testHubLogger{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/agent/channel", strings.NewReader(`{"action":"open","session_id":"direct-session","mode":"direct"}`))
	req.Header.Set("Authorization", "Token relay-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if upstreamCalled {
		t.Fatalf("hub must not forward non-relay channel payloads from relays")
	}
}

func TestRelayChannelStoreRecordsAndClosesSession(t *testing.T) {
	store := NewRelayChannelStore()
	open := store.Record("hash", RelayChannelSession{
		SessionID:  "relay-session",
		Mode:       "relay",
		Hostname:   "relay-1",
		Version:    "0.7.32",
		LastAction: "open",
	})
	closed := store.Record("hash", RelayChannelSession{
		SessionID:  "relay-session",
		Mode:       "relay",
		Hostname:   "relay-1",
		Version:    "0.7.32",
		LastAction: "close",
	})

	if open.ConnectedAt.IsZero() || closed.LastSeenAt.IsZero() || closed.ClosedAt.IsZero() {
		t.Fatalf("expected session timestamps, open=%#v closed=%#v", open, closed)
	}
	if len(store.Snapshot()) != 1 {
		t.Fatalf("expected one tracked session")
	}
}

type testHubLogger struct{}

func (testHubLogger) Info(string)          {}
func (testHubLogger) Error(string)         {}
func (testHubLogger) Fatal(string, ...any) {}
func (testHubLogger) Sync()                {}
