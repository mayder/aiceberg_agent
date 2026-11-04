package usecase

import (
	"context"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func boundedFlushConcurrency(n int) int {
	if n < 1 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

// Groups retain their route/auth/identity boundary and sequential chunks. Only independent groups run concurrently.
func (uc *FlushOutbox) flushGroups(ctx context.Context, groups map[flushGroupKey][]entities.Envelope) (int, int, int, error) {
	if uc.concurrency <= 1 || len(groups) < 2 {
		a, r, err := uc.flushGroupsSequential(groups)
		return a, r, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return uc.dispatchFlushGroups(ctx, groups)
}

type flushWork struct {
	key   flushGroupKey
	batch []entities.Envelope
}

type flushResult struct {
	acked, retained, deferred int
	err                       error
}

func (uc *FlushOutbox) dispatchFlushGroups(ctx context.Context, groups map[flushGroupKey][]entities.Envelope) (int, int, int, error) {
	jobs := make(chan flushWork, len(groups))
	results := make(chan flushResult, len(groups))
	workers := uc.concurrency
	if workers > len(groups) {
		workers = len(groups)
	}
	for i := 0; i < workers; i++ {
		go uc.runFlushWorker(ctx, jobs, results)
	}
	for key, batch := range groups {
		jobs <- flushWork{key, batch}
	}
	close(jobs)
	acked, retained, deferred := 0, 0, 0
	var firstErr error
	for range groups {
		r := <-results
		acked += r.acked
		retained += r.retained
		deferred += r.deferred
		firstErr = firstError(firstErr, r.err)
	}
	return acked, retained, deferred, firstErr
}

func (uc *FlushOutbox) runFlushWorker(ctx context.Context, jobs <-chan flushWork, results chan<- flushResult) {
	for item := range jobs {
		if err := ctx.Err(); err != nil {
			results <- flushResult{retained: len(item.batch), deferred: len(item.batch), err: err}
			continue
		}
		a, r, err := uc.flushGroupsSequential(map[flushGroupKey][]entities.Envelope{item.key: item.batch})
		results <- flushResult{acked: a, retained: r, err: err}
	}
}
