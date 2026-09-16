package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func batchJob(id int, host string) entities.AgentlessJob {
	return entities.AgentlessJob{CheckID: id, AtivoID: id, Tipo: "synthetic-unsupported", Endpoint: &entities.AgentlessEndpoint{Endereco: host}}
}

func TestAgentlessBatchBoundsConcurrencyAndSerializesDestination(t *testing.T) {
	jobs := []entities.AgentlessJob{}
	for i := 0; i < 40; i++ {
		jobs = append(jobs, batchJob(i, fmt.Sprintf("host-%d", i%10)))
	}
	var mu sync.Mutex
	active, peak, completed := 0, 0, 0
	hosts := map[string]bool{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	firstWorkers := make(chan struct{})
	err := runAgentlessBatch(ctx, jobs, 4, func(ctx context.Context, job entities.AgentlessJob) error {
		mu.Lock()
		if hosts[job.Endpoint.Endereco] {
			t.Errorf("overlap for %s", job.Endpoint.Endereco)
		}
		hosts[job.Endpoint.Endereco] = true
		active++
		if active > peak {
			peak = active
			if peak == 4 {
				close(firstWorkers)
			}
		}
		mu.Unlock()
		select {
		case <-firstWorkers:
		case <-ctx.Done():
			return ctx.Err()
		}
		mu.Lock()
		active--
		hosts[job.Endpoint.Endereco] = false
		completed++
		mu.Unlock()
		return nil
	})
	if err != nil || completed != 40 || peak != 4 {
		t.Fatalf("err=%v completed=%d peak=%d", err, completed, peak)
	}
}

func TestAgentlessBatchCancellationDoesNotStartQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	err := runAgentlessBatch(ctx, []entities.AgentlessJob{batchJob(1, "a"), batchJob(2, "b")}, 1, func(context.Context, entities.AgentlessJob) error { started.Add(1); cancel(); return nil })
	if !errors.Is(err, context.Canceled) || started.Load() != 1 {
		t.Fatalf("err=%v started=%d", err, started.Load())
	}
}

func TestAgentlessBatchPropagatesFailureAndStopsQueue(t *testing.T) {
	failure := errors.New("storage unavailable")
	var started int
	err := runAgentlessBatch(context.Background(), []entities.AgentlessJob{batchJob(1, "a"), batchJob(2, "b")}, 1, func(context.Context, entities.AgentlessJob) error { started++; return failure })
	if !errors.Is(err, failure) || started != 1 {
		t.Fatalf("err=%v started=%d", err, started)
	}
}

type batchOutbox struct {
	observations []entities.AgentlessObservation
	err          error
}

func (b *batchOutbox) Append(o entities.AgentlessObservation) error {
	if b.err != nil {
		return b.err
	}
	b.observations = append(b.observations, o)
	return nil
}
func (b *batchOutbox) ReadBatch(int) ([]entities.AgentlessObservation, error) {
	return b.observations, nil
}
func (b *batchOutbox) Ack([]string) error { return nil }
func (b *batchOutbox) Len() (int, int64)  { return len(b.observations), 0 }

func TestAgentlessParallelRunPersistsEveryResult(t *testing.T) {
	outbox := &batchOutbox{}
	uc := NewAgentlessHub(config.Config{}, testLogger{}, nil, outbox, nil, nil)
	for i := 1; i <= 100; i++ {
		uc.targets = append(uc.targets, batchJob(i, fmt.Sprintf("host-%d", i)))
	}
	if err := uc.PollAndRun(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(outbox.observations) != 100 {
		t.Fatalf("observations=%d", len(outbox.observations))
	}
	seen := map[int]bool{}
	for _, obs := range outbox.observations {
		if seen[obs.CheckID] {
			t.Fatalf("duplicate check %d", obs.CheckID)
		}
		seen[obs.CheckID] = true
	}
}

func TestAgentlessParallelRunReportsPersistenceFailure(t *testing.T) {
	failure := errors.New("outbox full")
	uc := NewAgentlessHub(config.Config{}, testLogger{}, nil, &batchOutbox{err: failure}, nil, nil)
	uc.targets = []entities.AgentlessJob{batchJob(1, "a")}
	if err := uc.PollAndRun(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
}

func BenchmarkAgentlessBatch(b *testing.B) {
	jobs := make([]entities.AgentlessJob, 64)
	for i := range jobs {
		jobs[i] = batchJob(i, fmt.Sprintf("destination-%d", i))
	}
	for _, workers := range []int{1, 8} {
		b.Run(fmt.Sprintf("workers_%d", workers), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				err := runAgentlessBatch(context.Background(), jobs, workers, func(ctx context.Context, _ entities.AgentlessJob) error {
					timer := time.NewTimer(5 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-timer.C:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
