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
	collector ports.Collector
	outbox    ports.OutboxRepo
	log       logger.Logger
}

func NewCollectAndBuffer(c ports.Collector, o ports.OutboxRepo, l logger.Logger) *CollectAndBuffer {
	return &CollectAndBuffer{collector: c, outbox: o, log: l}
}

func (uc *CollectAndBuffer) Execute(ctx context.Context) error {
	data, err := uc.collector.Collect(ctx) // []byte
	if err != nil {
		uc.log.Error("collect: " + err.Error())
		return err
	}

	hostname, _ := os.Hostname()
	env := entities.Envelope{
		ID:            genID(),
		SchemaVersion: 1,
		Kind:          "metric",
		Sub:           uc.collector.Name(),
		AgentID:       hostname,
		TSUnixMs:      time.Now().UnixMilli(),
		Body:          json.RawMessage(data), // mantém como JSON bruto
	}

	if err := uc.outbox.Append(env); err != nil {
		uc.log.Error("outbox append: " + err.Error())
		return err
	}
	uc.log.Info("buffered: " + env.ID)
	return nil
}

func genID() string { return time.Now().UTC().Format("20060102T150405.000000000") }
