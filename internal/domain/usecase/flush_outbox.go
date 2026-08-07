package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

const defaultMaxFlushBatchBytes = 8 * 1024 * 1024

type FlushOutbox struct {
	outbox        ports.OutboxRepo
	tx            ports.Transport
	log           logger.Logger
	defaultAuth   string
	onConfig      IngestConfigHandler
	batchSize     int
	maxBatchBytes int
	now           func() time.Time
	mu            sync.Mutex
	backoff       map[string]backoffState
	snapshot      FlushOutboxSnapshot
}

type FlushOutboxOptions struct {
	BatchSize     int
	MaxBatchBytes int
}

type FlushOutboxSnapshot struct {
	LastError             string         `json:"last_error,omitempty"`
	LastErrorRoute        string         `json:"last_error_route,omitempty"`
	LastErrorBatch        int            `json:"last_error_batch,omitempty"`
	LastAckRoute          string         `json:"last_ack_route,omitempty"`
	LastAckBatch          int            `json:"last_ack_batch,omitempty"`
	LastAckAtUnix         int64          `json:"last_ack_at_unix,omitempty"`
	LastDurationMs        int64          `json:"last_duration_ms,omitempty"`
	LastRetained          int            `json:"last_retained,omitempty"`
	OldestPendingAgeSec   int64          `json:"oldest_pending_age_sec,omitempty"`
	LastBackoffRoute      string         `json:"last_backoff_route,omitempty"`
	LastBackoffUntilUnix  int64          `json:"last_backoff_until_unix,omitempty"`
	LastSuggestedBatch    int            `json:"last_suggested_batch,omitempty"`
	LastBackpressureRoute string         `json:"last_backpressure_route,omitempty"`
	EndpointBacklog       map[string]int `json:"endpoint_backlog,omitempty"`
}

type backoffState struct {
	failures int
	until    time.Time
}

type flushGroupKey struct {
	auth     string
	identity string
	endpoint string
}

func NewFlushOutbox(o ports.OutboxRepo, t ports.Transport, l logger.Logger, defaultAuth string, onConfig IngestConfigHandler) *FlushOutbox {
	return NewFlushOutboxWithOptions(o, t, l, defaultAuth, onConfig, FlushOutboxOptions{})
}

func NewFlushOutboxWithOptions(o ports.OutboxRepo, t ports.Transport, l logger.Logger, defaultAuth string, onConfig IngestConfigHandler, opts FlushOutboxOptions) *FlushOutbox {
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 50
	}
	maxBatchBytes := opts.MaxBatchBytes
	if maxBatchBytes <= 0 {
		maxBatchBytes = defaultMaxFlushBatchBytes
	}
	return &FlushOutbox{
		outbox:        o,
		tx:            t,
		log:           l,
		defaultAuth:   defaultAuth,
		onConfig:      onConfig,
		batchSize:     batchSize,
		maxBatchBytes: maxBatchBytes,
		now:           time.Now,
		backoff:       make(map[string]backoffState),
	}
}

// Execute envia um lote da outbox; retorna quantos envelopes foram ackados.
func (uc *FlushOutbox) Execute(ctx context.Context) (int, error) {
	batch, err := uc.outbox.ReadBatch(uc.batchSize)
	if err != nil || len(batch) == 0 {
		return 0, err
	}

	start := time.Now()
	grouped, invalidIDs := uc.groupFlushBatch(batch)
	uc.updatePendingSnapshot(batch)

	if len(invalidIDs) > 0 {
		if err := uc.outbox.Ack(invalidIDs); err != nil {
			uc.log.Error(logger.KV("ack invalid envelopes failed",
				"err", err,
			))
		}
	}
	if len(grouped) == 0 {
		return 0, nil
	}

	acked, retained, firstErr := uc.flushGroups(grouped)

	durationMs := time.Since(start).Milliseconds()
	uc.log.Info(logger.KV("flushed",
		"batch_size", len(batch),
		"acked", acked,
		"retained", retained,
		"duration_ms", durationMs,
	))
	uc.recordSummary(durationMs, retained)
	return acked, firstErr
}

func (uc *FlushOutbox) flushGroups(grouped map[flushGroupKey][]entities.Envelope) (int, int, error) {
	var firstErr error
	acked, retained := 0, 0
	for group, list := range grouped {
		key := group.auth + "|" + group.identity + "|" + group.endpoint
		if until, ok := uc.backoffUntil(key); ok {
			retained += len(list)
			firstErr = firstError(firstErr, errors.New("transport backoff active"))
			uc.log.Info(logger.KV("transport backoff active", "route", group.endpoint,
				"batch_size", len(list), "retry_after_ms", time.Until(until).Milliseconds()))
			continue
		}
		chunks, err := splitFlushBatchByJSONSize(list, uc.maxBatchBytes)
		if err != nil {
			retained += len(list)
			uc.recordIngestAckFailure(group.endpoint, len(list), err)
			uc.registerFailure(key, group.endpoint, len(list), err)
			uc.log.Error(logger.KV("ingest batch exceeds safe request size",
				"route", group.endpoint,
				"batch_size", len(list),
				"max_batch_bytes", uc.maxBatchBytes,
				"err", err,
			))
			firstErr = firstError(firstErr, err)
			continue
		}
		for _, chunk := range chunks {
			chunkAcked, chunkRetained, err := uc.flushChunk(group, chunk)
			acked += chunkAcked
			retained += chunkRetained
			firstErr = firstError(firstErr, err)
		}
	}
	return acked, retained, firstErr
}

func (uc *FlushOutbox) groupFlushBatch(batch []entities.Envelope) (map[flushGroupKey][]entities.Envelope, []string) {
	grouped := make(map[flushGroupKey][]entities.Envelope)
	invalidIDs := make([]string, 0, 1)
	for _, envelope := range batch {
		if envelope.ID == "" {
			HandleInvalidEnvelope(uc.log, envelope, "missing_envelope_id")
			invalidIDs = append(invalidIDs, envelope.ID)
			continue
		}
		auth := envelope.AuthHeader
		if auth == "" {
			auth = uc.defaultAuth
		}
		endpoint := envelope.Endpoint
		if endpoint == "" {
			endpoint = "/v1/ingest"
		}
		key := flushGroupKey{auth: auth, identity: envelope.IdentityHeader, endpoint: endpoint}
		grouped[key] = append(grouped[key], envelope)
	}
	return grouped, invalidIDs
}

func firstError(current, candidate error) error {
	if current != nil {
		return current
	}
	return candidate
}

func (uc *FlushOutbox) flushChunk(group flushGroupKey, list []entities.Envelope) (int, int, error) {
	key := group.auth + "|" + group.identity + "|" + group.endpoint
	if until, ok := uc.backoffUntil(key); ok {
		uc.log.Info(logger.KV("transport backoff active",
			"route", group.endpoint, "batch_size", len(list),
			"retry_after_ms", time.Until(until).Milliseconds(),
		))
		return 0, len(list), errors.New("transport backoff active")
	}
	respBody, err := uc.tx.SendWithAuth(list, group.auth, group.endpoint)
	if err != nil {
		uc.recordTransportFailure(key, group.endpoint, len(list), err)
		return 0, len(list), err
	}
	uc.resetBackoff(key)
	uc.handleIngestConfig(group.auth, group.endpoint, respBody)
	uc.applyBackpressure(key, group.endpoint, respBody)
	if err := uc.validateIngestAck(group.endpoint, list, respBody); err != nil {
		return 0, len(list), err
	}
	ids := envelopeIDs(list)
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error(logger.KV("ack failed", "route", group.endpoint, "batch_size", len(ids), "err", err))
		return 0, len(list), err
	}
	uc.recordAck(group.endpoint, len(ids))
	return len(ids), 0, nil
}

func (uc *FlushOutbox) recordTransportFailure(key, endpoint string, batchSize int, err error) {
	uc.logTransportFailure(endpoint, batchSize, err)
	if delay, ok := uc.registerFailure(key, endpoint, batchSize, err); ok {
		uc.log.Info(logger.KV("transport backoff scheduled", "route", endpoint, "batch_size", batchSize,
			"err_kind", retry.ErrorKindTransient, "retry_after_ms", delay.Milliseconds()))
		return
	}
	uc.log.Error(logger.KV("transport permanent failure cooldown", "route", endpoint, "batch_size", batchSize,
		"err_kind", retry.ClassifyError(err), "retry_after_ms", retry.DefaultMaxBackoff.Milliseconds(), "err", err))
}

func (uc *FlushOutbox) handleIngestConfig(auth, endpoint string, respBody []byte) {
	if uc.onConfig == nil {
		return
	}
	cfg, err := parseIngestConfig(respBody)
	if err != nil {
		uc.log.Error(logger.KV("ingest config parse failed", "route", endpoint, "err", err))
	}
	if cfg != nil {
		uc.onConfig(auth, *cfg)
	}
}

func (uc *FlushOutbox) validateIngestAck(endpoint string, list []entities.Envelope, respBody []byte) error {
	ackResult, err := parseIngestAckResult(respBody)
	if err != nil {
		uc.recordIngestAckFailure(endpoint, len(list), err)
		uc.log.Error(logger.KV("ingest response parse failed", "route", endpoint, "batch_size", len(list), "err", err))
		return err
	}
	if ingestAckIsSafe(ackResult, len(list)) {
		return nil
	}
	err = errors.New("ingest response did not acknowledge batch")
	uc.recordIngestAckFailure(endpoint, len(list), err)
	uc.log.Error(logger.KV("ingest batch retained", "route", endpoint, "batch_size", len(list),
		"received", ackResult.Received, "skipped", ackResult.Skipped,
		"errors_by_reason", ackResult.ErrorsByReason, "err", err))
	return err
}

func splitFlushBatchByJSONSize(batch []entities.Envelope, maxBytes int) ([][]entities.Envelope, error) {
	chunks := make([][]entities.Envelope, 0, 1)
	current := make([]entities.Envelope, 0, len(batch))
	currentBytes := 2
	for _, envelope := range batch {
		raw, err := json.Marshal(envelope)
		if err != nil {
			return nil, fmt.Errorf("marshal envelope %q: %w", envelope.ID, err)
		}
		if len(raw)+2 > maxBytes {
			return nil, fmt.Errorf("envelope %q has %d bytes; limit is %d", envelope.ID, len(raw)+2, maxBytes)
		}
		separatorBytes := 0
		if len(current) > 0 {
			separatorBytes = 1
		}
		if currentBytes+separatorBytes+len(raw) > maxBytes {
			chunks = append(chunks, current)
			current = make([]entities.Envelope, 0, len(batch)-len(current))
			currentBytes = 2
			separatorBytes = 0
		}
		current = append(current, envelope)
		currentBytes += separatorBytes + len(raw)
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks, nil
}

func (uc *FlushOutbox) logTransportFailure(endpoint string, batchSize int, err error) {
	kind := retry.ClassifyError(err)
	fields := []any{
		"route", endpoint,
		"batch_size", batchSize,
		"err_kind", kind,
	}
	if se, ok := err.(interface{ StatusCode() int }); ok {
		fields = append(fields, "status", se.StatusCode())
	}
	fields = append(fields, "err", err)
	if kind == retry.ErrorKindTransient {
		uc.log.Info(logger.KV("transport transient failure", fields...))
		return
	}
	uc.log.Error(logger.KV("transport failed", fields...))
}

func (uc *FlushOutbox) Snapshot() FlushOutboxSnapshot {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	out := uc.snapshot
	if uc.snapshot.EndpointBacklog != nil {
		out.EndpointBacklog = make(map[string]int, len(uc.snapshot.EndpointBacklog))
		for k, v := range uc.snapshot.EndpointBacklog {
			out.EndpointBacklog[k] = v
		}
	}
	return out
}

func (uc *FlushOutbox) backoffUntil(key string) (time.Time, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	state := uc.backoff[key]
	if state.until.IsZero() || !uc.now().Before(state.until) {
		return time.Time{}, false
	}
	return state.until, true
}

func (uc *FlushOutbox) registerFailure(key, endpoint string, batchSize int, err error) (time.Duration, bool) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.snapshot.LastError = err.Error()
	uc.snapshot.LastErrorRoute = endpoint
	uc.snapshot.LastErrorBatch = batchSize
	if retry.ClassifyError(err) != retry.ErrorKindTransient {
		delay := retry.DefaultMaxBackoff
		state := uc.backoff[key]
		state.until = uc.now().Add(delay)
		uc.backoff[key] = state
		uc.snapshot.LastBackoffRoute = endpoint
		uc.snapshot.LastBackoffUntilUnix = state.until.Unix()
		return delay, false
	}
	state := uc.backoff[key]
	state.failures++
	delay := retry.BackoffDelay(state.failures, retry.DefaultInitialBackoff, retry.DefaultMaxBackoff, retry.DefaultMinJitter, retry.DefaultMaxJitter, nil)
	state.until = uc.now().Add(delay)
	uc.backoff[key] = state
	uc.snapshot.LastBackoffRoute = endpoint
	uc.snapshot.LastBackoffUntilUnix = state.until.Unix()
	return delay, true
}

func (uc *FlushOutbox) resetBackoff(key string) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	delete(uc.backoff, key)
}

func (uc *FlushOutbox) recordIngestAckFailure(endpoint string, batchSize int, err error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.snapshot.LastError = err.Error()
	uc.snapshot.LastErrorRoute = endpoint
	uc.snapshot.LastErrorBatch = batchSize
}

func (uc *FlushOutbox) recordAck(endpoint string, batchSize int) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.snapshot.LastAckRoute = endpoint
	uc.snapshot.LastAckBatch = batchSize
	uc.snapshot.LastAckAtUnix = uc.now().Unix()
}

func (uc *FlushOutbox) recordSummary(durationMs int64, retained int) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.snapshot.LastDurationMs = durationMs
	uc.snapshot.LastRetained = retained
}

func (uc *FlushOutbox) updatePendingSnapshot(batch []entities.Envelope) {
	now := uc.now()
	endpoints := make(map[string]int)
	var oldest int64
	for _, e := range batch {
		endpoint := strings.TrimSpace(e.Endpoint)
		if endpoint == "" {
			endpoint = "/v1/ingest"
		}
		endpoints[endpoint]++
		if e.TSUnixMs > 0 && (oldest == 0 || e.TSUnixMs < oldest) {
			oldest = e.TSUnixMs
		}
	}
	var ageSec int64
	if oldest > 0 {
		ageSec = int64(now.Sub(time.UnixMilli(oldest)).Seconds())
		if ageSec < 0 {
			ageSec = 0
		}
	}
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.snapshot.EndpointBacklog = endpoints
	uc.snapshot.OldestPendingAgeSec = ageSec
}

func (uc *FlushOutbox) applyBackpressure(key, endpoint string, body []byte) {
	bp, err := parseIngestBackpressure(body)
	if err != nil || bp == nil {
		return
	}
	target := strings.TrimSpace(bp.DegradedEndpoint)
	if target == "" {
		target = endpoint
	}
	if target != endpoint {
		return
	}
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if bp.RetryAfterSec > 0 {
		state := uc.backoff[key]
		state.until = uc.now().Add(time.Duration(bp.RetryAfterSec) * time.Second)
		uc.backoff[key] = state
		uc.snapshot.LastBackoffRoute = endpoint
		uc.snapshot.LastBackoffUntilUnix = state.until.Unix()
	}
	uc.snapshot.LastSuggestedBatch = bp.SuggestedBatchSize
	uc.snapshot.LastBackpressureRoute = endpoint
}

func envelopeIDs(batch []entities.Envelope) []string {
	ids := make([]string, 0, len(batch))
	for _, e := range batch {
		if e.ID != "" {
			ids = append(ids, e.ID)
		}
	}
	return ids
}
