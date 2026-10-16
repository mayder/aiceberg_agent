package usecase

import (
	"context"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"time"
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
func (uc *FlushOutbox) flushGroups(ctx context.Context, groups map[flushGroupKey][]entities.Envelope) (int, int, error) {
	if uc.concurrency <= 1 || len(groups) < 2 {
		return uc.flushGroupsSequential(groups)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	type work struct {
		key   flushGroupKey
		batch []entities.Envelope
	}
	type result struct {
		acked, retained int
		err             error
	}
	jobs := make(chan work, len(groups))
	results := make(chan result, len(groups))
	workers := uc.concurrency
	if workers > len(groups) {
		workers = len(groups)
	}
	for i := 0; i < workers; i++ {
		go func() {
			for item := range jobs {
				if err := ctx.Err(); err != nil {
					results <- result{retained: len(item.batch), err: err}
					continue
				}
				a, r, e := uc.flushGroupsSequential(map[flushGroupKey][]entities.Envelope{item.key: item.batch})
				results <- result{a, r, e}
			}
		}()
	}
	for key, batch := range groups {
		jobs <- work{key, batch}
	}
	close(jobs)
	acked, retained := 0, 0
	var firstErr error
	for range groups {
		r := <-results
		acked += r.acked
		retained += r.retained
		firstErr = firstError(firstErr, r.err)
	}
	return acked, retained, firstErr
}
