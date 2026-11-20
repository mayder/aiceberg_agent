package remote

import (
	"context"
	"encoding/json"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelaySNMPClientUsesOnlyHubAndRequiresExplicitAcceptance(t *testing.T) {
	cloudCalls, hubCalls := 0, 0
	cloud := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { cloudCalls++ }))
	defer cloud.Close()
	accepted := false
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hubCalls++
		if r.Method != "POST" || r.Header.Get("Authorization") != "Token fixture-relay-secret" || r.Header.Get("X-Agent-Identity") == "" {
			t.Error("missing authenticated relay request")
		}
		if r.URL.Path != "/v1/relay-snmp-probe/result" {
			t.Error("wrong fixed route")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["command_id"] != "fixture-command-0001" {
			t.Error("command reference lost")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "accepted": accepted})
	}))
	defer hub.Close()
	cfg := config.Config{APIBaseURL: cloud.URL, HubURL: hub.URL, AgentMode: "relay", AgentID: 424, AgentClientID: 3, Agent: config.AgentCfg{Token: "fixture-relay-secret"}}
	client := NewRelaySNMPProbeClient(cfg)
	observation := entities.AgentlessObservation{CommandID: "fixture-command-0001"}
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("unconfirmed result acknowledged")
	}
	accepted = true
	if err := client.ReportProbe(context.Background(), observation); err != nil {
		t.Fatal(err)
	}
	hub.Close()
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("offline HUB acknowledged")
	}
	if cloudCalls != 0 || hubCalls != 2 {
		t.Fatal("relay bypassed HUB or replayed report")
	}
	cfg.HubURL = ""
	client = NewRelaySNMPProbeClient(cfg)
	if err := client.ReportProbe(context.Background(), observation); err == nil {
		t.Fatal("missing HUB accepted")
	}
}
func TestRelaySNMPClientNeverReturnsRawHTTPBodyOrWrongExecutor(t *testing.T) {
	valid := false
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !valid {
			w.WriteHeader(403)
			_, _ = w.Write([]byte("fixture-snmp-secret"))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ready","executor_id":999,"job":{"cliente_id":3}}`))
	}))
	defer hub.Close()
	client := NewRelaySNMPProbeClient(config.Config{HubURL: hub.URL, AgentMode: "relay", AgentID: 424, AgentClientID: 3, Agent: config.AgentCfg{Token: "fixture-relay-key"}})
	if _, err := client.FetchProbe(context.Background(), "fixture-command-0001"); err == nil || strings.Contains(err.Error(), "fixture-snmp-secret") {
		t.Fatal("raw upstream error returned")
	}
	valid = true
	if _, err := client.FetchProbe(context.Background(), "fixture-command-0001"); err == nil {
		t.Fatal("wrong executor accepted")
	}
}
