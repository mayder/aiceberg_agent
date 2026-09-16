package usecase

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	local "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessFlushDrainsBoundedBatchesAndPreservesUnsentData(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(status) }))
			defer server.Close()
			store := local.NewMemStore()
			for i := 0; i < 20; i++ {
				if err := store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i)}); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.Config{APIBaseURL: server.URL, AgentlessFlushBatch: 2}
			uc := NewAgentlessHub(cfg, testLogger{}, remote.NewAgentlessHubClient(cfg), repositories.NewAgentlessOutboxRepository(store), nil, nil)
			err := uc.Flush(context.Background())
			count, _ := store.Len()
			if status == http.StatusOK {
				if err != nil || calls != 8 || count != 4 {
					t.Fatalf("err=%v calls=%d remaining=%d", err, calls, count)
				}
			} else if err == nil || calls != 1 || count != 20 {
				t.Fatalf("err=%v calls=%d remaining=%d", err, calls, count)
			}
		})
	}
}

func TestAgentlessBackpressureSkipsRoutineFetchWithoutDiscardingObservations(t *testing.T) {
	store := local.NewMemStore()
	for i := 0; i < 16; i++ {
		if err := store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i)}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Config{AgentlessFlushBatch: 2}
	// A nil client makes any accidental network attempt fail the test.
	uc := NewAgentlessHub(cfg, testLogger{}, nil, repositories.NewAgentlessOutboxRepository(store), nil, nil)
	uc.targets = []entities.AgentlessJob{batchJob(1, "a")}
	if err := uc.SyncTargets(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(uc.targetsSnapshot()) != 0 {
		t.Fatal("stale targets still scheduled")
	}
	count, _ := store.Len()
	if count != 16 {
		t.Fatalf("observations lost: %d", count)
	}
}
