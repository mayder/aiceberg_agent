package usecase

import (
	"context"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type FlushOutbox struct {
	outbox      ports.OutboxRepo
	tx          ports.Transport
	log         logger.Logger
	defaultAuth string
	onConfig    IngestConfigHandler
}

func NewFlushOutbox(o ports.OutboxRepo, t ports.Transport, l logger.Logger, defaultAuth string, onConfig IngestConfigHandler) *FlushOutbox {
	return &FlushOutbox{o, t, l, defaultAuth, onConfig}
}

// Execute envia um lote da outbox; retorna quantos envelopes foram ackados.
func (uc *FlushOutbox) Execute(ctx context.Context) (int, error) {
	batch, err := uc.outbox.ReadBatch(50)
	if err != nil || len(batch) == 0 {
		return 0, err
	}

	start := time.Now()
	grouped := make(map[string]map[string][]entities.Envelope) // auth -> endpoint -> list
	for _, e := range batch {
		h := e.AuthHeader
		if h == "" {
			h = uc.defaultAuth
		}
		end := e.Endpoint
		if end == "" {
			end = "/v1/ingest"
		}
		if grouped[h] == nil {
			grouped[h] = make(map[string][]entities.Envelope)
		}
		grouped[h][end] = append(grouped[h][end], e)
	}

	for auth, byEndpoint := range grouped {
		for endpoint, list := range byEndpoint {
			respBody, err := uc.tx.SendWithAuth(list, auth, endpoint)
			if err != nil {
				uc.log.Error("transport failed: " + err.Error())
				return 0, err
			}
			if uc.onConfig != nil {
				cfg, err := parseIngestConfig(respBody)
				if err != nil {
					uc.log.Error("ingest config parse failed: " + err.Error())
				}
				if cfg != nil {
					uc.onConfig(auth, *cfg)
				}
			}
		}
	}

	ids := make([]string, 0, len(batch))
	for _, e := range batch {
		ids = append(ids, e.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error("ack failed: " + err.Error())
		return 0, err
	}
	uc.log.Info("flushed ack=" + strconv.Itoa(len(ids)) + " duration_ms=" + strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	return len(ids), nil
}
