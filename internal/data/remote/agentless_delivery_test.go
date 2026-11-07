package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentlessAttemptTimingIsExplicitUTCWithoutMutatingObservation(t *testing.T) {
	var attempts []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Observations []map[string]any `json:"observations"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		attempts = append(attempts, body.Observations[0])
		fmt.Fprint(w, `{"status":"ok","saved":1,"failed":0}`)
	}))
	defer server.Close()
	at := time.Date(2026, 11, 6, 9, 0, 0, 123456789, time.FixedZone("local", -3*3600))
	o := entities.AgentlessObservation{ID: "persisted", CheckID: 7, ObservedAt: at, SegmentStartedAt: &at, DedupeKey: "immutable", Payload: map[string]any{"original": true}}
	client := NewAgentlessHubClient(config.Config{APIBaseURL: server.URL})
	for i := 0; i < 2; i++ {
		if err := client.SendObservations(context.Background(), []entities.AgentlessObservation{o}); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range attempts {
		timing := item["delivery_timing"].(map[string]any)
		if timing["version"] != "agentless_delivery_v1" || timing["segment_finished_at"] != "2026-11-06T12:00:00.123456789Z" {
			t.Fatalf("wrong origin: %v", timing)
		}
		if _, err := time.Parse(time.RFC3339Nano, timing["hub_send_started_at"].(string)); err != nil {
			t.Fatal(err)
		}
		if item["dedupe_key"] != "immutable" {
			t.Fatal("dedupe changed")
		}
	}
	if len(o.Payload) != 1 || o.ID != "persisted" || !o.ObservedAt.Equal(at) {
		t.Fatal("mutated queued observation")
	}
	if attempts[0]["delivery_timing"].(map[string]any)["hub_send_started_at"] == attempts[1]["delivery_timing"].(map[string]any)["hub_send_started_at"] {
		t.Fatal("retry reused attempt time")
	}
}

func TestAgentlessPartialServiceOrUnclassifiedFailureNeverIsolates(t *testing.T) {
	for _, class := range []string{"service", "", "payload"} {
		t.Run(class, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"status":"ok","saved":0,"failed":1,"failure_class":%q}`, class)
			}))
			defer server.Close()
			client := NewAgentlessHubClient(config.Config{APIBaseURL: server.URL})
			err := client.SendObservations(context.Background(), []entities.AgentlessObservation{{CheckID: 7, DedupeKey: "stable"}})
			delivery, ok := err.(*AgentlessDeliveryError)
			if !ok || delivery.PayloadRejected != (class == "payload") {
				t.Fatalf("incorrect retry classification: %v", err)
			}
		})
	}
}
