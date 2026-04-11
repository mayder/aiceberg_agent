package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessHubClientSendObservationsIncludesSegmentMeta(t *testing.T) {
	t.Helper()

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/hub-agentless/observations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	}))
	defer srv.Close()

	started := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	client := NewAgentlessHubClient(config.Config{APIBaseURL: srv.URL})
	err := client.SendObservations(context.Background(), []entities.AgentlessObservation{
		{
			CheckID:          77,
			Status:           "ok",
			Payload:          map[string]any{"stats": map[string]any{}},
			ObservedAt:       started,
			CollectionKind:   "switchport_slow",
			SegmentID:        "switchport_slow",
			SegmentSeq:       2,
			IsPartial:        true,
			IsFinal:          false,
			SegmentStartedAt: &started,
		},
	})
	if err != nil {
		t.Fatalf("send observations: %v", err)
	}

	items, ok := got["observations"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("observations inesperado: %#v", got["observations"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("observation invalida: %#v", items[0])
	}
	if item["collection_kind"] != "switchport_slow" || item["snmp_collection_kind"] != "switchport_slow" {
		t.Fatalf("collection_kind ausente: %#v", item)
	}
	if item["segment_id"] != "switchport_slow" {
		t.Fatalf("segment_id ausente: %#v", item)
	}
	if item["segment_seq"] != float64(2) {
		t.Fatalf("segment_seq inesperado: %#v", item["segment_seq"])
	}
	if item["is_partial"] != true || item["is_final"] != false {
		t.Fatalf("flags parciais inesperadas: %#v", item)
	}
	if item["segment_started_at"] != "2026-04-10 12:00:00" {
		t.Fatalf("segment_started_at inesperado: %#v", item["segment_started_at"])
	}
}
