package usecase

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessRoutineCapsLeasesThenStopsAtOutboxCapacity(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		if limit := req.URL.Query().Get("limit"); limit != "1" {
			t.Errorf("leased %q jobs when outbox had room for only one", limit)
		}
		_, _ = w.Write([]byte(`{"status":"ok","count":1,"jobs":[{"check_id":72,"tipo":"fixture"}]}`))
	}))
	defer api.Close()
	cfg := config.Config{APIBaseURL: api.URL, AgentlessJobsLimit: 50, AgentlessFlushBatch: 1}
	queue := &batchOutbox{observations: make([]entities.AgentlessObservation, agentlessFlushBatches-1)}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), queue, nil, nil)
	ctx := context.Background()

	if err := uc.SyncTargets(ctx); err != nil {
		t.Fatal(err)
	}
	// An earlier cached lease may include more jobs than the current free capacity.
	uc.targets = append(uc.targets, batchJob(73, "cached-a"), batchJob(74, "cached-b"))
	for range 2 {
		if err := uc.PollAndRun(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := queue.Len(); n != agentlessFlushBatches {
		t.Fatalf("cached routine admitted more observations than expected: %d", n)
	}
	if err := uc.SyncTargets(ctx); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("leased work despite backpressure: %d requests", n)
	}
}

func TestAgentlessManualWaitCanCancelBeforeLeasing(t *testing.T) {
	fetched := make(chan struct{}, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fetched <- struct{}{}
		_, _ = w.Write([]byte(`{"status":"ok","jobs":[]}`))
	}))
	defer api.Close()
	cfg := config.Config{APIBaseURL: api.URL}
	uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), &batchOutbox{}, nil, nil)
	if err := uc.acquireRun(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer uc.releaseRun()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- uc.CollectCommand(ctx, AgentlessCommandRequest{CommandID: "fixture", CheckIDs: []int{72}})
	}()
	select {
	case <-fetched:
		t.Fatal("manual check was leased before its execution slot opened")
	case <-time.After(60 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected waiting result: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("manual check stayed blocked after cancellation")
	}
}

func TestAgentlessCancelledNetworkProbeDoesNotPersistAnOfflineObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assertInterruptedAgentlessRequestDoesNotPersist(t, ctx, cancel, context.Canceled)
}

func TestAgentlessParentDeadlineDoesNotPersistAnOfflineObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	assertInterruptedAgentlessRequestDoesNotPersist(t, ctx, func() {}, context.DeadlineExceeded)
}

func assertInterruptedAgentlessRequestDoesNotPersist(t *testing.T, ctx context.Context, interrupt func(), expected error) {
	t.Helper()
	var contacted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		contacted.Store(true)
		interrupt()
		<-req.Context().Done()
	}))
	defer server.Close()
	queue := local.NewMemStore()
	uc := NewAgentlessHub(config.Config{}, testLogger{}, nil, repositories.NewAgentlessOutboxRepository(queue), nil, nil)
	uc.targets = []entities.AgentlessJob{{
		CheckID: 72, AtivoID: 3, ClienteID: 2, Tipo: "http", BaseIntervalSec: 60, TimeoutMs: 1000,
		Endpoint: &entities.AgentlessEndpoint{ID: 1, Tipo: "url", Endereco: server.URL},
	}}
	if err := uc.PollAndRun(ctx); !errors.Is(err, expected) || !contacted.Load() {
		t.Fatalf("fixture did not interrupt a real request: %v", err)
	}
	observations, err := queue.Peek(2)
	if err != nil || len(observations) != 0 {
		t.Fatalf("interrupted probe produced an availability observation: %v %v", observations, err)
	}
	if len(uc.probeAdmission.circuits) != 0 {
		t.Fatal("the parent deadline or cancellation changed the probe circuits")
	}
}
