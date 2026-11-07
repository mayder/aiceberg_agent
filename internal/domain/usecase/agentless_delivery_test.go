package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAgentlessPriorityIsolatesPoisonWithoutLosingOrOvertakingFinal(t *testing.T) {
	calls := 0
	seen := map[int]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Observations []struct {
				Check int `json:"check_id"`
				Seq   int `json:"segment_seq"`
			} `json:"observations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, o := range body.Observations {
			if o.Check == 1 {
				w.WriteHeader(422)
				return
			}
		}
		for _, o := range body.Observations {
			seen[o.Check]++
			if o.Seq == 2 {
				t.Error("final overtook rejected partial")
			}
		}
		fmt.Fprintf(w, `{"status":"ok","saved":%d,"failed":0}`, len(body.Observations))
	}))
	defer server.Close()
	store := local.NewMemStore()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		_ = store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i), CheckID: i, CollectionKind: "metrics_fast", DedupeKey: fmt.Sprint(i), SegmentSeq: 1, ObservedAt: at, SegmentStartedAt: &at})
	}
	_ = store.Push(entities.AgentlessObservation{ID: "final", CheckID: 1, CollectionKind: "metrics_fast", DedupeKey: "final", SegmentSeq: 2, IsFinal: true, ObservedAt: at, SegmentStartedAt: &at})
	cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 3, AgentlessDeliveryPriority: true}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	if err := uc.Flush(context.Background()); err == nil {
		t.Fatal("poison not reported")
	}
	count, _ := store.Len()
	if count != 2 || seen[2] != 1 || seen[3] != 1 || calls > 8 {
		t.Fatalf("remaining=%d seen=%v calls=%d", count, seen, calls)
	}
}

func TestAgentlessPriorityPreservesGlobalBackoffAmbiguousAckAndLegacy(t *testing.T) {
	for _, scenario := range []string{"401", "429", "503", "timeout", "partial", "invalid", "empty", "no_dedupe"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch scenario {
				case "401":
					w.WriteHeader(401)
				case "429":
					w.WriteHeader(429)
				case "503":
					w.WriteHeader(503)
				case "timeout":
					w.WriteHeader(504)
				case "partial":
					fmt.Fprint(w, `{"status":"ok","saved":1,"failed":1}`)
				case "invalid":
					fmt.Fprint(w, `not json`)
				case "no_dedupe":
					w.WriteHeader(422)
				}
			}))
			defer server.Close()
			store := local.NewMemStore()
			// No dedupe means a possibly partially accepted batch must never be split/replayed.
			for i := 1; i <= 2; i++ {
				_ = store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i), CheckID: i})
			}
			cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 2, AgentlessDeliveryPriority: true}
			uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
			if err := uc.Flush(context.Background()); err == nil {
				t.Fatal("unconfirmed request acknowledged")
			}
			count, _ := store.Len()
			if calls != 1 || count != 2 {
				t.Fatalf("calls=%d remaining=%d", calls, count)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := uc.Flush(ctx); err == nil || calls != 1 {
				t.Fatal("cancelled flush dispatched")
			}
		})
	}
}

func TestAgentlessPriorityAcknowledgesOnlyDurableSuccessAndRollbackRetainsBacklog(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"status":"ok","saved":1,"failed":0}`)
	}))
	defer server.Close()
	store := local.NewMemStore()
	_ = store.Push(entities.AgentlessObservation{ID: "legacy", CheckID: 1})
	cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 1, AgentlessDeliveryPriority: true}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	if err := uc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count, _ := store.Len(); count != 0 || calls != 1 {
		t.Fatal("ACK failed")
	}
	_ = store.Push(entities.AgentlessObservation{ID: "rollback", CheckID: 2})
	uc.cfg.AgentlessDeliveryPriority = false
	if err := uc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count, _ := store.Len(); count != 0 || calls != 2 {
		t.Fatal("rollback did not consume unchanged backlog")
	}
}

func TestAgentlessPriorityPartialAckReplaysOnlyWithStableDedupe(t *testing.T) {
	accepted := map[string]bool{}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Observations []struct {
				Check int    `json:"check_id"`
				Key   string `json:"dedupe_key"`
			} `json:"observations"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		failed, saved := 0, 0
		for _, o := range body.Observations {
			if o.Check == 1 {
				failed++
				continue
			}
			accepted[o.Key] = true
			saved++
		}
		fmt.Fprintf(w, `{"status":"ok","saved":%d,"failed":%d,"failure_class":"payload"}`, saved, failed)
	}))
	defer server.Close()
	store := local.NewMemStore()
	for i := 1; i <= 2; i++ {
		_ = store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i), CheckID: i, DedupeKey: fmt.Sprint(i)})
	}
	cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 2, AgentlessDeliveryPriority: true}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	if err := uc.Flush(context.Background()); err == nil {
		t.Fatal("partial failure not reported")
	}
	count, _ := store.Len()
	if count != 1 || len(accepted) != 1 || !accepted["2"] || calls > 8 {
		t.Fatalf("unsafe partial ACK remaining=%d accepted=%v calls=%d", count, accepted, calls)
	}
}

func TestAgentlessPriorityCancellationDuringTransportPreservesBatch(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		_ = json.NewDecoder(r.Body).Decode(&body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	store := local.NewMemStore()
	_ = store.Push(entities.AgentlessObservation{ID: "pending", CheckID: 1, DedupeKey: "stable"})
	cfg := config.Config{APIBaseURL: server.URL, AgentlessDeliveryPriority: true}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- uc.Flush(ctx) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request never dispatched")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled transport acknowledged")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("transport ignored cancellation")
	}
	if count, _ := store.Len(); count != 1 {
		t.Fatal("cancelled batch removed")
	}
}
