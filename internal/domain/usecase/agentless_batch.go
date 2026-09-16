package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const agentlessBatchWorkers = 8

// runAgentlessBatch bounds parallelism and serializes checks for the same destination.
// The first persistence error cancels remaining work; callers retain unacknowledged data.
func runAgentlessBatch(ctx context.Context, jobs []entities.AgentlessJob, workers int, run func(context.Context, entities.AgentlessJob) error) error {
	if workers < 1 {
		workers = 1
	}
	if workers > agentlessBatchWorkers {
		workers = agentlessBatchWorkers
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	groups := groupAgentlessJobs(jobs)
	queue := make(chan []entities.AgentlessJob)
	var wg sync.WaitGroup
	var firstErr error
	var once sync.Once
	for i := 0; i < workers && i < len(groups); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for group := range queue {
				for _, job := range group {
					if ctx.Err() != nil {
						return
					}
					if err := run(ctx, job); err != nil {
						once.Do(func() { firstErr = err; cancel() })
						return
					}
				}
			}
		}()
	}
	for _, group := range groups {
		select {
		case queue <- group:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(queue)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func groupAgentlessJobs(jobs []entities.AgentlessJob) [][]entities.AgentlessJob {
	groups := make([][]entities.AgentlessJob, 0)
	positions := make(map[string]int)
	for _, job := range jobs {
		key := fmt.Sprintf("asset:%d:%d", job.ClienteID, job.AtivoID)
		if job.Endpoint != nil && strings.TrimSpace(job.Endpoint.Endereco) != "" {
			key = strings.ToLower(strings.TrimSpace(job.Endpoint.Endereco))
		}
		index, ok := positions[key]
		if !ok {
			index = len(groups)
			positions[key] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], job)
	}
	return groups
}
