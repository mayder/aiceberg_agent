package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// Called under flushMu. At most eight HTTP attempts, including isolation splits.
func (uc *AgentlessHub) flushPriority(ctx context.Context) error {
	reader, ok := uc.outbox.(ports.AgentlessDeliveryReader)
	if !ok {
		return errors.New("agentless priority reader unavailable")
	}
	excluded := map[string]bool{}
	var pending [][]entities.AgentlessObservation
	var lastError error
	for attempt := 0; attempt < agentlessFlushBatches; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch, rest, err := uc.nextDeliveryBatch(reader, pending, excluded)
		if err != nil {
			return err
		}
		pending = rest
		batch = eligibleDelivery(batch, excluded)
		if len(batch) == 0 {
			if len(pending) > 0 {
				continue
			}
			return lastError
		}
		err = uc.sendDelivery(ctx, batch)
		if err != nil {
			lastError = err
			var delivery *remote.AgentlessDeliveryError
			if !errors.As(err, &delivery) || !delivery.PayloadRejected || !dedupableDelivery(batch) {
				return err
			}
			if len(batch) > 1 {
				middle := len(batch) / 2
				pending = append([][]entities.AgentlessObservation{batch[:middle], batch[middle:]}, pending...)
			} else {
				excluded[entities.AgentlessDeliveryStream(batch[0])] = true
			}
			continue
		}
	}
	return lastError
}

func eligibleDelivery(batch []entities.AgentlessObservation, excluded map[string]bool) []entities.AgentlessObservation {
	out := make([]entities.AgentlessObservation, 0, len(batch))
	for _, o := range batch {
		if !excluded[entities.AgentlessDeliveryStream(o)] {
			out = append(out, o)
		}
	}
	return out
}

func dedupableDelivery(batch []entities.AgentlessObservation) bool {
	for _, o := range batch {
		if o.DedupeKey == "" {
			return false
		}
	}
	return true
}

func (uc *AgentlessHub) sendDelivery(ctx context.Context, batch []entities.AgentlessObservation) error {
	started := time.Now()
	err := uc.client.SendObservations(ctx, batch)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		uc.log.Error(logger.KV("agentless delivery deferred", "job_id", batch[0].CheckID, "job_type", "flush", "status", "deferred", "latency_ms", duration, "batch_size", len(batch), "err", err))
		return err
	}
	ids := make([]string, 0, len(batch))
	for _, o := range batch {
		ids = append(ids, o.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		return err
	}
	uc.log.Info(logger.KV("agentless delivery acknowledged", "job_id", batch[0].CheckID, "job_type", "flush", "status", "ok", "latency_ms", duration, "batch_size", len(ids)))
	return nil
}

func (uc *AgentlessHub) nextDeliveryBatch(reader ports.AgentlessDeliveryReader, pending [][]entities.AgentlessObservation, excluded map[string]bool) ([]entities.AgentlessObservation, [][]entities.AgentlessObservation, error) {
	if len(pending) > 0 {
		return pending[0], pending[1:], nil
	}
	batch, err := reader.ReadDeliveryBatch(uc.flushBatchSize(), uc.deliveryOrdinal, excluded)
	if err == nil {
		uc.deliveryOrdinal += uint64(len(batch))
	}
	return batch, pending, err
}
