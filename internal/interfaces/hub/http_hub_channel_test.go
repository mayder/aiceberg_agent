package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestHubChannelForwardsRelayPresenceToAiceberg(t *testing.T) {
	var upstreamAuth string
	var upstreamIdentity string
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/channel" {
			t.Fatalf("unexpected upstream route %s", r.URL.Path)
		}
		upstreamAuth = r.Header.Get("Authorization")
		upstreamIdentity = r.Header.Get("X-Agent-Identity")
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
	req.Header.Set("X-Agent-Identity", "relay-identity")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamAuth != "Token relay-token" {
		t.Fatalf("expected relay auth forwarded, got %q", upstreamAuth)
	}
	if upstreamIdentity != "relay-identity" {
		t.Fatalf("expected relay identity forwarded, got %q", upstreamIdentity)
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

func TestHubIngestPreservesRelayIdentityHeader(t *testing.T) {
	outbox := &testHubOutbox{}
	handler := NewHandler(config.Config{}, outbox, testHubLogger{}, nil)

	body := `[{"envelope_id":"env-1","agent_id":"relay-node","schema_version":1,"kind":"metric","ts_unix_ms":1,"body":{"ok":true}}]`
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/metrics", strings.NewReader(body))
	req.Header.Set("Authorization", "Token relay-token")
	req.Header.Set("X-Agent-Identity", "relay-identity")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(outbox.batch) != 1 {
		t.Fatalf("expected 1 buffered envelope, got %d", len(outbox.batch))
	}
	env := outbox.batch[0]
	if env.AuthHeader != "Token relay-token" {
		t.Fatalf("expected auth preserved, got %q", env.AuthHeader)
	}
	if env.IdentityHeader != "relay-identity" {
		t.Fatalf("expected identity preserved, got %q", env.IdentityHeader)
	}
}

func TestHubProxyPreservesIdentityForBootstrapConfigAndPing(t *testing.T) {
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"|"+r.Method+"|"+r.Header.Get("Authorization")+"|"+r.Header.Get("X-Agent-Identity"))
		switch r.URL.Path {
		case "/v1/agent/config":
			_, _ = w.Write([]byte(`{"version":"cfg-1","collect":{}}`))
		case "/v1/agent/bootstrap":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/v1/agent/ping":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"challenge":"c1"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			t.Fatalf("unexpected upstream route %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := NewHandler(config.Config{APIBaseURL: upstream.URL}, nil, testHubLogger{}, nil)
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/v1/agent/config"},
		{method: http.MethodPost, path: "/v1/agent/bootstrap", body: `{"hostname":"relay-1"}`},
		{method: http.MethodGet, path: "/v1/agent/ping"},
		{method: http.MethodPost, path: "/v1/agent/ping", body: `{"challenge":"c1"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Token relay-token")
		req.Header.Set("X-Agent-Identity", "relay-identity")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("%s %s returned %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	if len(seen) != 4 {
		t.Fatalf("expected 4 upstream calls, got %#v", seen)
	}
	for _, call := range seen {
		if !strings.Contains(call, "|Token relay-token|relay-identity") {
			t.Fatalf("identity/auth not preserved in %q", call)
		}
	}
}

func TestHubProxyForwardsAgentControlRoutes(t *testing.T) {
	var seen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"|"+r.Method+"|"+r.Header.Get("Authorization")+"|"+r.Header.Get("X-Agent-Identity"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","commands":[]}`))
	}))
	defer upstream.Close()

	handler := NewHandler(config.Config{APIBaseURL: upstream.URL}, nil, testHubLogger{}, nil)
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/v1/agent/selfheal-commands"},
		{method: http.MethodPost, path: "/v1/agent/selfheal-report", body: `{"command_id":"cmd-1","status":"success"}`},
		{method: http.MethodPost, path: "/v1/agent/error-report", body: `{"errors":[]}`},
		{method: http.MethodPost, path: "/v1/agent/update-report", body: `{"status":"precheck_ok"}`},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Token relay-token")
		req.Header.Set("X-Agent-Identity", "relay-identity")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code >= 300 {
			t.Fatalf("%s %s returned %d body=%s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}

	if len(seen) != 4 {
		t.Fatalf("expected 4 upstream calls, got %#v", seen)
	}
	for _, call := range seen {
		if !strings.Contains(call, "|Token relay-token|relay-identity") {
			t.Fatalf("identity/auth not preserved in %q", call)
		}
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

type testHubOutbox struct {
	batch []entities.Envelope
}

func (o *testHubOutbox) Append(env entities.Envelope) error {
	o.batch = append(o.batch, env)
	return nil
}

func (o *testHubOutbox) ReadBatch(n int) ([]entities.Envelope, error) {
	if n > len(o.batch) {
		n = len(o.batch)
	}
	out := make([]entities.Envelope, n)
	copy(out, o.batch[:n])
	return out, nil
}

func (o *testHubOutbox) Ack(ids []string) error { return nil }

func (o *testHubOutbox) Len() (int, int64) { return len(o.batch), 0 }
