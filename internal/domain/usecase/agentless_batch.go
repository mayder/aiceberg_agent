package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const agentlessBatchWorkers = 8

type agentlessBatchPolicy struct {
	workers     int
	limit       int
	admission   *agentlessProbeAdmission
	manual      bool
	outboxEmpty bool
}

type agentlessBatchResult struct {
	work    agentlessBatchWork
	started time.Time
	outcome agentlessProbeOutcome
}

type agentlessBatchStats struct {
	dispatched      int
	circuitDeferred int
	budgetDeferred  int
	observationCost int
}

type agentlessJobRunner func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error)

// The dispatcher holds rate-limited jobs outside the workers. A persistence
// error stops new work immediately and cancels probes already running.
func runAgentlessBatch(ctx context.Context, jobs []entities.AgentlessJob, policy agentlessBatchPolicy, run agentlessJobRunner) (agentlessBatchStats, error) {
	limit := min(max(0, policy.limit), len(jobs))
	workers := min(max(1, policy.workers), agentlessBatchWorkers, limit)
	if workers == 0 {
		return agentlessBatchStats{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	schedule := newAgentlessBatchSchedule(jobs, workers, policy.limit)
	schedule.admission, schedule.manual = policy.admission, policy.manual
	schedule.allowOversize = policy.outboxEmpty
	schedule.admission.prune(time.Now())
	queue := make(chan agentlessBatchWork, workers)
	results := make(chan agentlessBatchResult, workers)
	var firstErr error
	var failOnce sync.Once
	var pool sync.WaitGroup
	for range workers {
		pool.Go(func() {
			workAgentlessBatch(ctx, queue, results, run, func(err error) {
				failOnce.Do(func() {
					firstErr = err
					cancel()
				})
			})
		})
	}

	count := dispatchAgentlessBatch(ctx, schedule, queue, results, workers, limit)
	close(queue)
	pool.Wait()
	stats := agentlessBatchStats{dispatched: count, circuitDeferred: schedule.circuitDeferred,
		budgetDeferred: schedule.budgetDeferred, observationCost: schedule.reserved}
	if firstErr != nil {
		return stats, firstErr
	}
	return stats, ctx.Err()
}

func dispatchAgentlessBatch(ctx context.Context, schedule *agentlessBatchSchedule, queue chan<- agentlessBatchWork, results <-chan agentlessBatchResult, workers, limit int) int {
	active, dispatched := 0, 0
	for {
		for ctx.Err() == nil && active < workers && dispatched < limit {
			work, ok := schedule.take(time.Now())
			if !ok {
				break
			}
			queue <- work
			active++
			dispatched++
		}
		if active < workers && dispatched < limit && ctx.Err() == nil && !schedule.wakeAt.IsZero() {
			result, ready := waitForAgentlessWorker(ctx, results, schedule.wakeAt)
			if ready {
				schedule.complete(result)
				active--
			}
			continue
		}
		if active == 0 {
			return dispatched
		}
		schedule.complete(<-results)
		active--
	}
}

func waitForAgentlessWorker(ctx context.Context, results <-chan agentlessBatchResult, due time.Time) (agentlessBatchResult, bool) {
	timer := time.NewTimer(time.Until(due))
	defer timer.Stop()
	select {
	case result := <-results:
		return result, true
	case <-timer.C:
	case <-ctx.Done():
	}
	return agentlessBatchResult{}, false
}

func (s *agentlessBatchSchedule) complete(result agentlessBatchResult) {
	s.release(result.work)
	now := time.Now()
	destination := result.work.destination.key
	s.admission.finishRate(destination, result.started, now)
	if result.started.IsZero() {
		return
	}
	if !s.manual {
		s.admission.recordExecution(result.work.job, destination, result.outcome, now)
	}
}

func workAgentlessBatch(ctx context.Context, queue <-chan agentlessBatchWork, results chan<- agentlessBatchResult, run agentlessJobRunner, fail func(error)) {
	for work := range queue {
		result := agentlessBatchResult{work: work}
		if ctx.Err() == nil {
			result.started = time.Now()
			outcome, err := run(ctx, work.job)
			if err != nil {
				fail(err)
			} else {
				result.outcome = outcome
			}
		}
		results <- result
	}
}
