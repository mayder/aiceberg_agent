package usecase

import (
	"context"
	"strconv"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type FlushOutbox struct {
	outbox ports.OutboxRepo
	tx     ports.Transport
	log    logger.Logger
}

func NewFlushOutbox(o ports.OutboxRepo, t ports.Transport, l logger.Logger) *FlushOutbox {
	return &FlushOutbox{o, t, l}
}

func (uc *FlushOutbox) Execute(ctx context.Context) error {
	batch, err := uc.outbox.ReadBatch(50)
	if err != nil || len(batch) == 0 {
		return err
	}
	if err := uc.tx.Send(batch); err != nil {
		uc.log.Error("transport: " + err.Error())
		return err
	}

	ids := make([]string, 0, len(batch))
	for _, e := range batch {
		ids = append(ids, e.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error("ack: " + err.Error())
		return err
	}
	uc.log.Info("flushed: ack=" + strconv.Itoa(len(ids)))
	return nil
}
