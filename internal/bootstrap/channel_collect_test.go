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
			"collect_now": []any{"inventory", "health", "shell", "network_capture"},
		},
	})
	want := []string{"inventory", "health", "network_capture"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected collect_now list: %#v", got)
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
