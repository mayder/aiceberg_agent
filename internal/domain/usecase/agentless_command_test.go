package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestCollectCommandDoesNotEnterRoutineTargetsOrDisk(t *testing.T) {
	var deliveries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/hub-agentless/jobs":
			if req.URL.Query().Get("command_id") != "manual-now" {
				t.Error("missing explicit command scope")
			}
			_, _ = w.Write([]byte(`{"status":"ok","jobs":[
				{"check_id":17,"tipo":"fixture"},
				{"check_id":72,"tipo":"fixture","command_id":"manual-old"},
				{"check_id":72,"tipo":"fixture","command_id":"manual-now","correlation_id":"wrong"},
				{"check_id":91,"tipo":"fixture","command_id":"manual-now","correlation_id":"trace-now"},
				{"check_id":72,"tipo":"fixture","command_id":"manual-now","correlation_id":"trace-now"},
				{"check_id":72,"tipo":"fixture","command_id":"manual-now","correlation_id":"trace-now"}
			]}`))
		case "/v1/hub-agentless/observations":
			var batch struct {
				Observations []struct {
					CheckID   int    `json:"check_id"`
					CommandID string `json:"command_id"`
				} `json:"observations"`
			}
			if err := json.NewDecoder(req.Body).Decode(&batch); err != nil || len(batch.Observations) != 1 ||
				batch.Observations[0].CheckID != 72 || batch.Observations[0].CommandID != "manual-now" {
				t.Error("manual observation did not reach its command")
			}
			deliveries.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok","saved":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store := local.NewTargetsStore(filepath.Join(t.TempDir(), "targets.json"))
	previous := []entities.AgentlessJob{{CheckID: 17, Tipo: "fixture"}, {CheckID: 91, Tipo: "fixture", CommandID: "manual-old"}}
	if err := store.Save(previous); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{APIBaseURL: server.URL}
	outbox := local.NewMemStore()
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(outbox), nil, store)
	ctx := context.Background()
	if err := uc.CollectCommand(ctx, AgentlessCommandRequest{CommandID: "manual-now", CorrelationID: "trace-now", CheckIDs: []int{72}}); err != nil {
		t.Fatal(err)
	}
	if err := uc.CollectCommand(ctx, AgentlessCommandRequest{CommandID: "manual-now", CorrelationID: "trace-now", CheckIDs: []int{73}}); err == nil {
		t.Fatal("manual command succeeded without an eligible check")
	}
	if err := uc.PollAndRun(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.Load()
	if err != nil || len(jobs) != 2 || jobs[0].CheckID != 17 || jobs[1].CommandID != "manual-old" {
		t.Fatal("manual run rewrote the routine target cache")
	}
	queued, err := outbox.Peek(4)
	if err != nil || len(queued) != 1 || queued[0].CheckID != 17 || queued[0].CommandID != "" || deliveries.Load() != 1 {
		t.Fatal("routine reused a current or historical manual command")
	}
}

func TestCollectCommandRetriesQueuedObservationWithoutCollectingAgain(t *testing.T) {
	var pulls, deliveries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/v1/hub-agentless/jobs":
			pulls.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok","jobs":[{"check_id":72,"tipo":"fixture","command_id":"manual-now"}]}`))
		case "/v1/hub-agentless/observations":
			if deliveries.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"status":"ok","saved":0,"failed":1,"failure_class":"service"}`))
			} else {
				_, _ = w.Write([]byte(`{"status":"ok","saved":1}`))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	cfg := config.Config{APIBaseURL: server.URL}
	outbox := local.NewMemStore()
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(outbox), nil, nil)
	ctx := context.Background()
	var pending *AgentlessPendingDeliveryError
	if err := uc.CollectCommand(ctx, AgentlessCommandRequest{CommandID: "manual-now", CheckIDs: []int{72}}); !errors.As(err, &pending) {
		t.Fatalf("retryable delivery did not preserve the command: %v", err)
	}
	if queued, _ := outbox.Peek(1); len(queued) != 1 || queued[0].CommandID != "manual-now" {
		t.Fatal("retryable failure removed the pending observation")
	}
	if err := uc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if queued, _ := outbox.Peek(1); len(queued) != 0 || pulls.Load() != 1 || deliveries.Load() != 2 {
		t.Fatal("the retry did not drain exactly one observation from a single collection")
	}
}
