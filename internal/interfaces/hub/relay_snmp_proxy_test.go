package hub

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
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
	if recorder.Code != 200 || calls != 1 || strings.Contains(recorder.Body.String(), "fixture-hub-key") ||
		recorder.Header().Get("Cache-Control") != "no-store, private" {
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
	}{{"GET", "{}", "fixture", 405}, {"POST", "{}", "", 401}, {"POST", strings.Repeat("x", 16385), "fixture", 400},
		{"POST", `{"command_id":"fixture-command-0001","credential_mode":"reference-v1"}`, "fixture", 503},
		{"POST", `{"command_id":"fixture-command-0001","operation":"resolve_snmp_profile","credential_mode":42}`, "fixture", 503}} {
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

func TestHubAssignmentProxyKeepsResultAndReceiptAcrossTLS(t *testing.T) {
	const body = `{"observation":{"check_id":72,"dedupe_key":"fixture-result","status":"ok","is_final":true}}`
	const receipt = `{"status":"ok","accepted":true,"dedupe_key":"fixture-result"}`
	const fenced = `{"status":"fenced","dedupe_key":"fixture-result"}`
	var results atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil || string(raw) != body || r.TLS == nil || r.URL.Path != relayprobe.AssignmentResultPath || r.URL.RawQuery != "" {
			t.Error("result changed between HUB and upstream")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		auth, identity := r.Header.Get("Authorization"), r.Header.Get("X-Agent-Identity")
		proof := relayprobe.Proof("fixture-hub-key", auth, r.URL.Path, raw, r.Header.Get("X-Aiceberg-Hub-Time"))
		if auth != "Token fixture-relay-key" || identity != "fixture-identity" || r.Header.Get("X-Aiceberg-Hub-Proof") != proof {
			t.Error("upstream did not receive the scoped relay proof")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch results.Add(1) {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, fenced)
		default:
			_, _ = io.WriteString(w, receipt)
		}
	}))
	defer upstream.Close()

	proxy := relaySNMPProxy(config.Config{AgentMode: "hub", APIBaseURL: upstream.URL, Agent: config.AgentCfg{Token: "fixture-hub-key"}},
		relayprobe.AssignmentResultPath, upstream.Client())
	gateway := httptest.NewTLSServer(proxy)
	defer gateway.Close()
	for _, test := range []struct {
		query, body string
		status      int
	}{
		{"?check_id=73", "assignment query parameters forbidden\n", http.StatusForbidden},
		{"", "", http.StatusServiceUnavailable},
		{"", fenced, http.StatusConflict},
		{"", receipt, http.StatusOK},
	} {
		req, err := http.NewRequest(http.MethodPost, gateway.URL+relayprobe.AssignmentResultPath+test.query, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Token fixture-relay-key")
		req.Header.Set("X-Agent-Identity", "fixture-identity")
		req.Header.Set("X-Aiceberg-Hub-Proof", "supplied-by-relay")
		res, err := gateway.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil || res.StatusCode != test.status || string(raw) != test.body ||
			(test.query == "" && res.Header.Get("Cache-Control") != "no-store, private") {
			t.Fatalf("forwarded receipt: status=%d body=%q: %v", res.StatusCode, raw, err)
		}
	}
	if results.Load() != 3 {
		t.Fatalf("unexpected upstream traffic: %d requests", results.Load())
	}

	request := httptest.NewRequest(http.MethodPost, relayprobe.AssignmentResultPath, strings.NewReader(body))
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || results.Load() != 3 {
		t.Fatal("cleartext request reached the upstream")
	}
}

func TestHubAssignmentRoutesRequireOptIn(t *testing.T) {
	cfg := config.Config{AgentMode: "hub", Agent: config.AgentCfg{Token: "fixture-hub-key"}}
	for _, scenario := range []struct {
		gate string
		want int
	}{
		{"", http.StatusNotFound},
		{"true", http.StatusForbidden}, // Both routes are present, and reject cleartext.
	} {
		t.Setenv("RELAY_SNMP_ASSIGNMENT_ENABLED", scenario.gate)
		handler := NewHandler(cfg, nil, testHubLogger{}, nil)
		for _, path := range []string{relayprobe.AssignmentJobPath, relayprobe.AssignmentResultPath} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
			reply := httptest.NewRecorder()
			handler.ServeHTTP(reply, req)
			if reply.Code != scenario.want {
				t.Fatalf("assignment gate %q, %s: got %d, want %d", scenario.gate, path, reply.Code, scenario.want)
			}
		}
	}
}
