package hub

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHubSNMPProxySignsExactBodyAndRelayWithoutExposingHubToken(t *testing.T) {
	calls := 0
	body := `{"command_id":"fixture-command-0001"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		auth := r.Header.Get("Authorization")
		stamp := r.Header.Get("X-Aiceberg-Hub-Time")
		if auth != "Token fixture-relay-key" || string(raw) != body || r.Header.Get("X-Agent-Identity") != "fixture-identity" {
			t.Error("relay identity or payload changed")
		}
		value, err := strconv.ParseInt(stamp, 10, 64)
		if err != nil || time.Now().Unix()-value > 1 {
			t.Error("invalid proof time")
		}
		if r.Header.Get("X-Aiceberg-Hub-Proof") != relayprobe.Proof("fixture-hub-key", auth, r.URL.Path, raw, stamp) {
			t.Error("invalid HUB proof")
		}
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer upstream.Close()
	handler := NewHandler(config.Config{AgentMode: "hub", APIBaseURL: upstream.URL, Agent: config.AgentCfg{Token: "fixture-hub-key"}}, nil, testHubLogger{}, nil)
	request := httptest.NewRequest("POST", relayprobe.JobPath, strings.NewReader(body))
	request.Header.Set("Authorization", "Token fixture-relay-key")
	request.Header.Set("X-Agent-Identity", "fixture-identity")
	request.Header.Set("X-Aiceberg-Hub-Proof", "forged")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != 200 || calls != 1 || strings.Contains(recorder.Body.String(), "fixture-hub-key") {
		t.Fatal("proxy failed or exposed token")
	}
}
func TestHubSNMPProxyRejectsMissingIdentityWrongMethodAndOversizedRequest(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer upstream.Close()
	handler := NewHandler(config.Config{AgentMode: "hub", APIBaseURL: upstream.URL, Agent: config.AgentCfg{Token: "fixture-hub-key"}}, nil, testHubLogger{}, nil)
	for _, tc := range []struct {
		method, body, identity string
		status                 int
	}{{"GET", "{}", "fixture", 405}, {"POST", "{}", "", 401}, {"POST", strings.Repeat("x", 16385), "fixture", 400}} {
		request := httptest.NewRequest(tc.method, relayprobe.JobPath, strings.NewReader(tc.body))
		request.Header.Set("Authorization", "Token fixture-relay-key")
		request.Header.Set("X-Agent-Identity", tc.identity)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != tc.status {
			t.Fatalf("wrong status %d", recorder.Code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request forwarded")
	}
}
