package usecase

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type CollectAndBuffer struct {
	collector  ports.Collector
	outbox     ports.OutboxRepo
	log        logger.Logger
	authHeader string
	endpoint   string
}

type BufferedCollectResult struct {
	EventID    string
	AgentID    string
	Endpoint   string
	Collector  string
	Body       []byte
	DurationMs int64
}

func NewCollectAndBuffer(c ports.Collector, o ports.OutboxRepo, l logger.Logger, authHeader string, endpoint string) *CollectAndBuffer {
	return &CollectAndBuffer{collector: c, outbox: o, log: l, authHeader: authHeader, endpoint: endpoint}
}

func (uc *CollectAndBuffer) Execute(ctx context.Context) error {
	_, err := uc.ExecuteDetailed(ctx)
	return err
}

func (uc *CollectAndBuffer) ExecuteDetailed(ctx context.Context) (*BufferedCollectResult, error) {
	start := time.Now()
	data, err := uc.collector.Collect(ctx) // []byte
	if err != nil {
		uc.log.Error(logger.KV("collect failed",
			"collector", uc.collector.Name(),
			"err", err,
		))
		return nil, err
	}

	hostname, _ := os.Hostname()
	if data == nil {
		uc.log.Info(logger.KV("collect empty",
			"collector", uc.collector.Name(),
		))
		return nil, nil
	}
	env := entities.Envelope{
		ID:            genID(),
		SchemaVersion: 1,
		Kind:          "metric",
		Sub:           uc.collector.Name(),
		AgentID:       hostname,
		TSUnixMs:      time.Now().UnixMilli(),
		Body:          json.RawMessage(data), // mantém como JSON bruto
		AuthHeader:    uc.authHeader,
		Endpoint:      uc.endpoint,
	}

	if env.ID == "" {
		HandleInvalidEnvelope(uc.log, env, "missing_envelope_id")
		return nil, nil
	}
	if err := uc.outbox.Append(env); err != nil {
		uc.log.Error(logger.KV("outbox append failed",
			"event_id", env.ID,
			"agent_id", env.AgentID,
			"route", env.Endpoint,
			"err", err,
		))
		return nil, err
	}
	durationMs := time.Since(start).Milliseconds()
	uc.log.Info(logger.KV("collect buffered",
		"event_id", env.ID,
		"agent_id", env.AgentID,
		"route", env.Endpoint,
		"duration_ms", durationMs,
	))
	return &BufferedCollectResult{
		EventID:    env.ID,
		AgentID:    env.AgentID,
		Endpoint:   env.Endpoint,
		Collector:  uc.collector.Name(),
		Body:       data,
		DurationMs: durationMs,
	}, nil
}

func genID() string { return time.Now().UTC().Format("20060102T150405.000000000") }
