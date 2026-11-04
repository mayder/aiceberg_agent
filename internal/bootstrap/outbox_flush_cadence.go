package app

import (
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

const hubOutboxCatchupDelay = time.Second

// Only progress with a full batch or undispatched work requests a short next turn. Failed
// requests keep their own cooldown; cancellation and no progress never spin.
func outboxCatchupDelay(mode string, cfg config.Config, acked int, snapshot usecase.FlushOutboxSnapshot, ctxErr error) time.Duration {
	enabled := (mode == "hub" && cfg.HubOutboxCatchupEnabled) || (mode == "relay" && cfg.RelayOutboxAsyncEnabled)
	if !enabled || ctxErr != nil || acked <= 0 {
		return 0
	}
	fullBatch := snapshot.BatchLimit > 0 && snapshot.LastReadBatch >= snapshot.BatchLimit
	if snapshot.LastDeferred <= 0 && !fullBatch {
		return 0
	}
	if cfg.OutboxFlushInterval <= hubOutboxCatchupDelay {
		return 0
	}
	return hubOutboxCatchupDelay
}

type outboxFlushCadence struct {
	lastPrune time.Time
}

// Catch-up must not accelerate the existing retention scans or pruning.
func (c *outboxFlushCadence) pruneDue(now time.Time, interval time.Duration) bool {
	if !c.lastPrune.IsZero() && now.Before(c.lastPrune.Add(interval)) {
		return false
	}
	c.lastPrune = now
	return true
}
