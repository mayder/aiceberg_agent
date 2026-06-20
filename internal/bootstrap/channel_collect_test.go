package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/channel"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestChannelEnvelopeCollectNowFiltersAllowedNames(t *testing.T) {
	got := channelEnvelopeCollectNow(channel.Envelope{
		Payload: map[string]any{
			"code":        "collect_now",
			"collect_now": []any{"inventory", "health", "shell", "network_capture", "log_source_discovery"},
		},
	})
	want := []string{"inventory", "health", "network_capture", "log_source_discovery"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected collect_now list: %#v", got)
	}
}

func TestChannelEnvelopeCollectNowAcceptsAgentlessCheckScope(t *testing.T) {
	got := channelEnvelopeCollectNow(channel.Envelope{
		Payload: map[string]any{
			"code":           "collect_now",
			"scope":          "agentless_check",
			"check_ids":      []any{101.0, "202", 0, "202"},
			"command_id":     "cmd-agentless",
			"correlation_id": "corr-agentless",
			"timeout_ms":     45000,
		},
	})
	if strings.Join(got, ",") != "agentless" {
		t.Fatalf("unexpected collect_now list: %#v", got)
	}

	req := channelEnvelopeAgentlessCommand(channel.Envelope{
		CommandID:     "env-command",
		CorrelationID: "env-correlation",
		Payload: map[string]any{
			"command_id":     "cmd-agentless",
			"correlation_id": "corr-agentless",
			"check_ids":      []any{101.0, "202", 0, "202"},
			"timeout_ms":     45000,
		},
	})
	if req.CommandID != "cmd-agentless" || req.CorrelationID != "corr-agentless" {
		t.Fatalf("refs operacionais inesperadas: %#v", req)
	}
	if len(req.CheckIDs) != 2 || req.CheckIDs[0] != 101 || req.CheckIDs[1] != 202 {
		t.Fatalf("check_ids inesperado: %#v", req.CheckIDs)
	}
	if req.TimeoutMs != 45000 {
		t.Fatalf("timeout_ms inesperado: %d", req.TimeoutMs)
	}
}

func TestSendCollectChunksReportsProgressAndResult(t *testing.T) {
	var received []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agent/channel" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		payload := map[string]any{}
		_ = json.Unmarshal(raw, &payload)
		received = append(received, payload)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	client := usecase.NewAgentChannelClient(config.Config{
		APIBaseURL: srv.URL,
		AgentMode:  "direct",
		Agent:      config.AgentCfg{Token: "agent-token"},
	}, logger.New("test"))
	sendCollectChunks(context.Background(), client, usecase.ControlCommand{
		Name:          "inventory",
		CommandID:     "collect-1",
		CorrelationID: "corr-1",
		Source:        "channel",
	}, &usecase.BufferedCollectResult{
		EventID:    "event-1",
		Endpoint:   "/v1/ingest/inventory",
		Collector:  "sysmetrics_inventory",
		Body:       []byte(strings.Repeat("a", 60*1024)),
		DurationMs: 12,
	})

	if len(received) < 2 {
		t.Fatalf("expected chunk progress and result events, got %d", len(received))
	}
	if received[0]["type"] != channel.TypeProgress {
		t.Fatalf("expected progress event, got %#v", received[0]["type"])
	}
	last := received[len(received)-1]
	if last["type"] != channel.TypeResult {
		t.Fatalf("expected result event, got %#v", last["type"])
	}
	result, ok := last["result"].(map[string]any)
	if !ok || result["fallback"] != "ingest_outbox" {
		t.Fatalf("expected ingest fallback result, got %#v", last["result"])
	}
}
