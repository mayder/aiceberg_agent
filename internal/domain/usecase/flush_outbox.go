package usecase

import (
	"context"
	"strconv"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type FlushOutbox struct {
	outbox      ports.OutboxRepo
	tx          ports.Transport
	log         logger.Logger
	defaultAuth string
}

func NewFlushOutbox(o ports.OutboxRepo, t ports.Transport, l logger.Logger, defaultAuth string) *FlushOutbox {
	return &FlushOutbox{o, t, l, defaultAuth}
}

func (uc *FlushOutbox) Execute(ctx context.Context) error {
	batch, err := uc.outbox.ReadBatch(50)
	if err != nil || len(batch) == 0 {
		return err
	}

	grouped := make(map[string][]entities.Envelope)
	for _, e := range batch {
		h := e.AuthHeader
		if h == "" {
			h = uc.defaultAuth
		}
		grouped[h] = append(grouped[h], e)
	}

	for auth, list := range grouped {
		if err := uc.tx.SendWithAuth(list, auth); err != nil {
			uc.log.Error("transport: " + err.Error())
			return err
		}
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
