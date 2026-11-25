package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	stats, err := runAgentlessBatch(ctx, jobs, agentlessBatchPolicy{workers: 4, limit: len(jobs)}, func(ctx context.Context, job entities.AgentlessJob) (agentlessProbeOutcome, error) {
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
			return agentlessProbeIndeterminate, ctx.Err()
		}
		mu.Lock()
		active--
		hosts[job.Endpoint.Endereco] = false
		completed++
		mu.Unlock()
		return agentlessProbeReachable, nil
	})
	if err != nil || stats.dispatched != 40 || completed != 40 || peak != 4 {
		t.Fatalf("err=%v completed=%d peak=%d", err, completed, peak)
	}
}

func TestAgentlessBatchCancellationDoesNotStartQueuedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	_, err := runAgentlessBatch(ctx, []entities.AgentlessJob{batchJob(1, "a"), batchJob(2, "b")}, agentlessBatchPolicy{workers: 1, limit: 2}, func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
		started.Add(1)
		cancel()
		return agentlessProbeIndeterminate, nil
	})
	if !errors.Is(err, context.Canceled) || started.Load() != 1 {
		t.Fatalf("err=%v started=%d", err, started.Load())
	}
}

func TestAgentlessBatchReservesSlotsForAllPossibleSNMPParts(t *testing.T) {
	snmp := batchJob(10, "segmented-fixture")
	snmp.Tipo = "snmp"
	snmp.SNMP = &entities.AgentlessSnmpProfile{Version: "v2c", ProfileID: 1}
	snmp.Config = entities.AgentlessConfig{"flush_partial": true, "groups": []string{"system", "interfaces_status"}}
	jobs := []entities.AgentlessJob{snmp, batchJob(11, "light-a"), batchJob(12, "light-b")}
	for _, scenario := range []struct {
		name     string
		budget   int
		empty    bool
		ids      []int
		reserved int
	}{
		{"skip expensive while pending", 2, false, []int{11, 12}, 2},
		{"admit final and two partials", 4, false, []int{10, 11}, 4},
		{"oversized probe runs alone when empty", 2, true, []int{10}, 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var ran []int
			stats, err := runAgentlessBatch(context.Background(), jobs,
				agentlessBatchPolicy{workers: 1, limit: scenario.budget, outboxEmpty: scenario.empty},
				func(_ context.Context, job entities.AgentlessJob) (agentlessProbeOutcome, error) {
					ran = append(ran, job.CheckID)
					return agentlessProbeReachable, nil
				},
			)
			if err != nil || !slices.Equal(ran, scenario.ids) || stats.observationCost != scenario.reserved {
				t.Fatalf("ran=%v reserved=%d error=%v", ran, stats.observationCost, err)
			}
		})
	}
}

func TestAgentlessBatchPropagatesFailureAndStopsQueue(t *testing.T) {
	failure := errors.New("storage unavailable")
	var started int
	_, err := runAgentlessBatch(context.Background(), []entities.AgentlessJob{batchJob(1, "a"), batchJob(2, "b")}, agentlessBatchPolicy{workers: 1, limit: 2}, func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
		started++
		return agentlessProbeIndeterminate, failure
	})
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
				_, err := runAgentlessBatch(context.Background(), jobs, agentlessBatchPolicy{workers: workers, limit: len(jobs)}, func(ctx context.Context, _ entities.AgentlessJob) (agentlessProbeOutcome, error) {
					timer := time.NewTimer(5 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-timer.C:
						return agentlessProbeReachable, nil
					case <-ctx.Done():
						return agentlessProbeIndeterminate, ctx.Err()
					}
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestAgentlessSchedulerRotatesTenantsAndSerializesAliasesWithinEach(t *testing.T) {
	first := batchJob(1, "https://[::ffff:192.0.2.20]:443/dashboard")
	alias := batchJob(2, "192.0.2.20:161")
	other := batchJob(3, "192.0.2.20")
	first.ClienteID, alias.ClienteID, other.ClienteID = 1, 1, 2
	schedule := newAgentlessBatchSchedule([]entities.AgentlessJob{first, alias, other}, 4, 3)
	gotFirst, ok := schedule.take(time.Now())
	if !ok || gotFirst.job.CheckID != 1 {
		t.Fatal("first tenant lost its first check")
	}
	gotOther, ok := schedule.take(time.Now())
	if !ok || gotOther.job.CheckID != 3 {
		t.Fatal("independent tenant was blocked by the same destination address")
	}
	if _, ok := schedule.take(time.Now()); ok {
		t.Fatal("two aliases of a physical destination were admitted concurrently")
	}
	schedule.release(gotFirst)
	if next, ok := schedule.take(time.Now()); !ok || next.job.CheckID != 2 {
		t.Fatal("release did not admit the waiting check")
	}
}

func TestAgentlessSchedulerReservesWorkersForCompetingClients(t *testing.T) {
	jobs := make([]entities.AgentlessJob, 0, 10)
	for i := 1; i <= 8; i++ {
		job := batchJob(i, fmt.Sprintf("high-volume-%d", i))
		job.ClienteID = 1
		jobs = append(jobs, job)
	}
	for i := 9; i <= 10; i++ {
		job := batchJob(i, "single-endpoint")
		job.ClienteID = 2
		jobs = append(jobs, job)
	}
	schedule := newAgentlessBatchSchedule(jobs, 4, len(jobs))
	first, ok1 := schedule.take(time.Now())
	other, ok2 := schedule.take(time.Now())
	second, ok3 := schedule.take(time.Now())
	if !ok1 || !ok2 || !ok3 || first.job.ClienteID != 1 || other.job.ClienteID != 2 || second.job.ClienteID != 1 {
		t.Fatal("the hot client's queue delayed the competing client")
	}
	if _, ok := schedule.take(time.Now()); ok {
		t.Fatal("competing tenant's reserved capacity was consumed")
	}
	schedule.release(other)
	if next, ok := schedule.take(time.Now()); !ok || next.job.CheckID != 10 {
		t.Fatal("the competing tenant did not resume when its endpoint became free")
	}
	if next, ok := schedule.take(time.Now()); !ok || next.job.ClienteID != 1 {
		t.Fatal("unused client capacity was not made available")
	}
}
