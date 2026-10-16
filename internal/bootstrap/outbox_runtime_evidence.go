package app

import (
	"strings"

	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func withOutboxRuntimeEvidence(snapshot map[string]any, repo ports.OutboxRepo, flush *usecase.FlushOutbox) map[string]any {
	snapshot["outbox_runtime"] = buildOutboxRuntimeEvidence(repo, flush)
	return snapshot
}

// Only bounded operational facts leave the host; raw transport errors can contain payloads.
func buildOutboxRuntimeEvidence(repo ports.OutboxRepo, flush *usecase.FlushOutbox) map[string]any {
	items, bytes := repo.Len()
	s := flush.Snapshot()
	return map[string]any{
		"queue_items": items, "queue_bytes": bytes,
		"last_error_kind":  outboxErrorKind(s.LastError),
		"last_error_route": s.LastErrorRoute, "last_error_batch": s.LastErrorBatch,
		"last_ack_route": s.LastAckRoute, "last_ack_batch": s.LastAckBatch,
		"last_ack_at_unix": s.LastAckAtUnix, "last_duration_ms": s.LastDurationMs,
		"last_retained": s.LastRetained, "oldest_pending_age_sec": s.OldestPendingAgeSec,
		"last_backoff_route": s.LastBackoffRoute, "last_backoff_until_unix": s.LastBackoffUntilUnix,
		"endpoint_backlog": s.EndpointBacklog,
	}
}

func outboxErrorKind(message string) string {
	switch {
	case message == "":
		return "none"
	case strings.Contains(message, "did not acknowledge"):
		return "ingest_ack_rejected"
	case strings.Contains(message, "timeout") || strings.Contains(message, "deadline"):
		return "timeout"
	case strings.Contains(message, "401"):
		return "http_401"
	case strings.Contains(message, "403"):
		return "http_403"
	case strings.Contains(message, "413"):
		return "http_413"
	case strings.Contains(message, "429"):
		return "http_429"
	case strings.Contains(message, "500"):
		return "http_500"
	case strings.Contains(message, "502"):
		return "http_502"
	case strings.Contains(message, "503"):
		return "http_503"
	case strings.Contains(message, "backoff"):
		return "backoff"
	case strings.Contains(message, "exceeds"):
		return "payload_limit"
	default:
		return "other"
	}
}
