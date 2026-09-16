package usecase

import (
	"context"

	"github.com/you/aiceberg_agent/internal/common/logger"
)

const agentlessFlushBatches = 8

func (uc *AgentlessHub) flushBatchSize() int {
	size := uc.getSettings().FlushBatch
	if size <= 0 {
		size = uc.cfg.AgentlessFlushBatch
	}
	if size <= 0 {
		size = 50
	}
	return size
}

func (uc *AgentlessHub) Flush(ctx context.Context) error {
	uc.flushMu.Lock()
	defer uc.flushMu.Unlock()
	for i := 0; i < agentlessFlushBatches; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := uc.flushBatch(ctx, uc.flushBatchSize())
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
	}
	return nil
}

func (uc *AgentlessHub) flushBatch(ctx context.Context, batchSize int) (int, error) {
	batch, err := uc.outbox.ReadBatch(batchSize)
	if err != nil || len(batch) == 0 {
		return 0, err
	}
	if err := uc.client.SendObservations(ctx, batch); err != nil {
		uc.log.Error(logger.KV("agentless send failed",
			"batch_size", len(batch),
			"err", err,
		))
		return 0, err
	}
	ids := make([]string, 0, len(batch))
	for _, o := range batch {
		ids = append(ids, o.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error(logger.KV("agentless outbox ack failed",
			"batch_size", len(ids),
			"err", err,
		))
		return 0, err
	}
	if uc.cfg.AgentlessDebug {
		uc.log.Info(logger.KV("agentless flushed batch",
			"batch_size", len(ids),
		))
	}
	uc.log.Info(logger.KV("agentless flushed ack",
		"batch_size", len(ids),
	))
	return len(batch), nil
}
