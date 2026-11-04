package app

import (
	"context"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type outboxCycleResult struct {
	acked     int
	failed    bool
	nextDelay time.Duration
}

type outboxFlushCycle struct {
	mode    string
	cfg     config.Config
	main    *usecase.FlushOutbox
	oslogs  *usecase.FlushOutbox
	counter *obsCounters
	prune   func()
	report  func(context.Context, string, string, string, error, map[string]any)
	cadence outboxFlushCadence
}

func (c *outboxFlushCycle) Execute(ctx context.Context) outboxCycleResult {
	var result outboxCycleResult
	if ctx.Err() != nil {
		return result
	}
	boundedPruning := (c.mode == "hub" && c.cfg.HubOutboxCatchupEnabled) || asyncOutboxEnabled(c.mode, c.cfg)
	if c.prune != nil && (!boundedPruning || c.cadence.pruneDue(time.Now(), c.cfg.OutboxFlushInterval)) {
		c.prune()
	}
	for _, target := range []struct {
		flush                   *usecase.FlushOutbox
		errorType, level, route string
	}{{c.main, "flush_outbox_failed", "error", "/v1/ingest"},
		{c.oslogs, "flush_oslogs_failed", "warning", "/v1/logs/raw"}} {
		if target.flush == nil || ctx.Err() != nil {
			continue
		}
		n, err := c.flushOne(ctx, target.flush, target.errorType, target.level, target.route)
		result.acked += n
		result.failed = result.failed || err != nil
		if delay := outboxCatchupDelay(c.mode, c.cfg, n, target.flush.Snapshot(), ctx.Err()); delay > 0 {
			result.nextDelay = delay
		}
	}
	return result
}

func (c *outboxFlushCycle) flushOne(ctx context.Context, flush *usecase.FlushOutbox, errorType, level, route string) (int, error) {
	start := time.Now()
	n, err := flush.Execute(ctx)
	if err != nil {
		c.counter.flushErr.Add(1)
		c.report(ctx, errorType, level, "open", err, map[string]any{"route": route})
	} else {
		c.counter.flushOK.Add(1)
		c.counter.lastFlushBatch.Store(int64(n))
		c.report(ctx, errorType, "info", "recovered", nil, map[string]any{"route": route})
	}
	c.counter.lastFlushMs.Store(time.Since(start).Milliseconds())
	return n, err
}

// Arrival-triggered Relay startup must not bring retention forward.
func (c *outboxFlushCycle) startRelayPruneCadence(now time.Time) {
	if c.mode == "relay" && c.cfg.RelayOutboxAsyncEnabled {
		c.cadence.lastPrune = now
	}
}
