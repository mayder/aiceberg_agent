package usecase

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
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
}

func NewCollectAndBuffer(c ports.Collector, o ports.OutboxRepo, l logger.Logger, authHeader string) *CollectAndBuffer {
	return &CollectAndBuffer{collector: c, outbox: o, log: l, authHeader: authHeader}
}

func (uc *CollectAndBuffer) Execute(ctx context.Context) error {
	start := time.Now()
	data, err := uc.collector.Collect(ctx) // []byte
	if err != nil {
		uc.log.Error("collect failed: " + err.Error())
		return err
	}

	hostname, _ := os.Hostname()
	if data == nil {
		uc.log.Info("collect empty")
		return nil
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
	}

	if err := uc.outbox.Append(env); err != nil {
		uc.log.Error("outbox append failed: " + err.Error())
		return err
	}
	uc.log.Info("collect buffered id=" + env.ID + " duration_ms=" + strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	return nil
}

func genID() string { return time.Now().UTC().Format("20060102T150405.000000000") }
