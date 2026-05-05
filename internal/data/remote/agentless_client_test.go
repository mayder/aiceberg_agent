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
			CommandID:        "cmd-pkg36",
			CorrelationID:    "corr-pkg36",
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
	if item["command_id"] != "cmd-pkg36" || item["correlation_id"] != "corr-pkg36" {
		t.Fatalf("refs operacionais ausentes: %#v", item)
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

func TestAgentlessHubClientSendObservationsSerializaPerfilProprietario(t *testing.T) {
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

	client := NewAgentlessHubClient(config.Config{APIBaseURL: srv.URL})
	err := client.SendObservations(context.Background(), []entities.AgentlessObservation{
		{
			CheckID: 77,
			Status:  "ok",
			Payload: map[string]any{
				"vendor_profile_applied": true,
				"profile_key":            "ale_oaw_ap_base_v1",
				"profile_version":        "1.0.0",
				"cpu_percent":            12,
				"memory_percent":         64,
				"cpu_memory": map[string]any{
					"source_profile": "ale_oaw_ap_base_v1",
					"vendor_samples": []map[string]any{
						{
							"name":          "ap_cpu_utilization",
							"oid":           "1.3.6.1.4.1.6486.1.1.0",
							"canonical_key": "cpu_percent",
							"source_mib":    "OAW-APxxxx",
							"value":         12,
						},
					},
				},
				"oids_success": []map[string]any{
					{"oid": "1.3.6.1.4.1.6486.1.1.0", "name": "ok_oid", "value": 12},
				},
				"oids_failed": []map[string]any{
					{"oid": "1.3.6.1.4.1.6486.1.2.0", "name": "fail_oid", "error": "sem retorno"},
				},
				"fallback_used": true,
			},
			ObservedAt: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
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
	payload, ok := item["payload_json"].(map[string]any)
	if !ok {
		t.Fatalf("payload_json inesperado: %#v", item["payload_json"])
	}
	if payload["vendor_profile_applied"] != true || payload["profile_key"] != "ale_oaw_ap_base_v1" || payload["profile_version"] != "1.0.0" || payload["fallback_used"] != true {
		t.Fatalf("metadados do perfil nao serializados: %#v", payload)
	}
	if payload["cpu_percent"] != float64(12) || payload["memory_percent"] != float64(64) {
		t.Fatalf("cpu/mem canonicos nao serializados: %#v", payload)
	}
	cpuMemory, ok := payload["cpu_memory"].(map[string]any)
	if !ok || cpuMemory["source_profile"] != "ale_oaw_ap_base_v1" {
		t.Fatalf("cpu_memory nao serializado: %#v", payload["cpu_memory"])
	}
	samples, ok := cpuMemory["vendor_samples"].([]any)
	if !ok || len(samples) != 1 {
		t.Fatalf("vendor_samples nao serializado: %#v", cpuMemory["vendor_samples"])
	}
	if success, ok := payload["oids_success"].([]any); !ok || len(success) != 1 {
		t.Fatalf("oids_success nao serializado: %#v", payload["oids_success"])
	}
	if failed, ok := payload["oids_failed"].([]any); !ok || len(failed) != 1 {
		t.Fatalf("oids_failed nao serializado: %#v", payload["oids_failed"])
	}
}
