package usecase

import (
	"context"
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
	validIDs := make([]string, 0, len(batch))
	invalidIDs := make([]string, 0, 1)
	for _, e := range batch {
		if e.ID == "" {
			HandleInvalidEnvelope(uc.log, e, "missing_envelope_id")
			invalidIDs = append(invalidIDs, e.ID)
			continue
		}
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
		validIDs = append(validIDs, e.ID)
	}

	if len(invalidIDs) > 0 {
		if err := uc.outbox.Ack(invalidIDs); err != nil {
			uc.log.Error(logger.KV("ack invalid envelopes failed",
				"err", err,
			))
		}
	}
	if len(validIDs) == 0 {
		return 0, nil
	}

	for auth, byEndpoint := range grouped {
		for endpoint, list := range byEndpoint {
			respBody, err := uc.tx.SendWithAuth(list, auth, endpoint)
			if err != nil {
				if se, ok := err.(interface{ StatusCode() int }); ok {
					uc.log.Error(logger.KV("transport failed",
						"route", endpoint,
						"batch_size", len(list),
						"status", se.StatusCode(),
						"err", err,
					))
				} else {
					uc.log.Error(logger.KV("transport failed",
						"route", endpoint,
						"batch_size", len(list),
						"err", err,
					))
				}
				return 0, err
			}
			if uc.onConfig != nil {
				cfg, err := parseIngestConfig(respBody)
				if err != nil {
					uc.log.Error(logger.KV("ingest config parse failed",
						"route", endpoint,
						"err", err,
					))
				}
				if cfg != nil {
					uc.onConfig(auth, *cfg)
				}
			}
		}
	}

	if err := uc.outbox.Ack(validIDs); err != nil {
		uc.log.Error(logger.KV("ack failed",
			"batch_size", len(validIDs),
			"err", err,
		))
		return 0, err
	}
	durationMs := time.Since(start).Milliseconds()
	uc.log.Info(logger.KV("flushed",
		"batch_size", len(validIDs),
		"duration_ms", durationMs,
	))
	return len(validIDs), nil
}
