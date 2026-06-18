package usecase

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	agentruntime "github.com/you/aiceberg_agent/internal/domain/runtime"
)

type CollectAndBuffer struct {
	collector      ports.Collector
	outbox         ports.OutboxRepo
	log            logger.Logger
	authHeader     string
	identityHeader string
	endpoint       string
	extraEndpoints func() []string
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

func NewCollectAndBufferWithIdentity(c ports.Collector, o ports.OutboxRepo, l logger.Logger, authHeader string, identityHeader string, endpoint string) *CollectAndBuffer {
	return &CollectAndBuffer{collector: c, outbox: o, log: l, authHeader: authHeader, identityHeader: identityHeader, endpoint: endpoint}
}

func NewCollectAndBufferWithIdentityAndExtraEndpoints(c ports.Collector, o ports.OutboxRepo, l logger.Logger, authHeader string, identityHeader string, endpoint string, extraEndpoints []string) *CollectAndBuffer {
	uc := NewCollectAndBufferWithIdentity(c, o, l, authHeader, identityHeader, endpoint)
	uc.extraEndpoints = func() []string { return extraEndpoints }
	return uc
}

func NewCollectAndBufferWithIdentityAndExtraEndpointsProvider(c ports.Collector, o ports.OutboxRepo, l logger.Logger, authHeader string, identityHeader string, endpoint string, extraEndpoints func() []string) *CollectAndBuffer {
	uc := NewCollectAndBufferWithIdentity(c, o, l, authHeader, identityHeader, endpoint)
	uc.extraEndpoints = extraEndpoints
	return uc
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
	data, err = agentruntime.WithPayloadMetadata(data, uc.collector.Name(), uc.endpoint)
	if err != nil {
		uc.log.Error(logger.KV("collect payload metadata failed",
			"collector", uc.collector.Name(),
			"route", uc.endpoint,
			"err", err,
		))
		return nil, err
	}
	env := entities.Envelope{
		ID:             genID(),
		SchemaVersion:  1,
		Kind:           "metric",
		Sub:            uc.collector.Name(),
		AgentID:        hostname,
		TSUnixMs:       time.Now().UnixMilli(),
		Body:           json.RawMessage(data), // mantém como JSON bruto
		AuthHeader:     uc.authHeader,
		IdentityHeader: uc.identityHeader,
		Endpoint:       uc.endpoint,
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
	for _, endpoint := range sanitizeExtraEndpoints(uc.endpoint, uc.currentExtraEndpoints()) {
		extra := env
		extra.ID = genID()
		extra.Endpoint = endpoint
		if err := uc.outbox.Append(extra); err != nil {
			uc.log.Error(logger.KV("outbox append failed",
				"event_id", extra.ID,
				"agent_id", extra.AgentID,
				"route", extra.Endpoint,
				"err", err,
			))
			return nil, err
		}
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

func (uc *CollectAndBuffer) currentExtraEndpoints() []string {
	if uc.extraEndpoints == nil {
		return nil
	}
	return uc.extraEndpoints()
}

func sanitizeExtraEndpoints(primary string, endpoints []string) []string {
	out := make([]string, 0, len(endpoints))
	seen := map[string]bool{primary: true}
	for _, endpoint := range endpoints {
		clean := strings.TrimSpace(endpoint)
		if !isSafeExtraEndpoint(clean) || seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}

func isSafeExtraEndpoint(endpoint string) bool {
	return strings.HasPrefix(endpoint, "/v1/logs/") &&
		!strings.Contains(endpoint, "://") &&
		!strings.ContainsAny(endpoint, " \t\r\n")
}
