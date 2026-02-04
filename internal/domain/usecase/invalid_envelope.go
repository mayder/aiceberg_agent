package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// HandleInvalidEnvelope centralizes handling for envelopes we refuse to send.
func HandleInvalidEnvelope(log logger.Logger, env entities.Envelope, reason string) {
	if log == nil {
		return
	}
	metrics.IncInvalidEnvelope()
	log.Error(logger.KV("invalid envelope dropped",
		"reason", reason,
		"event_id", env.ID,
		"agent_id", env.AgentID,
		"route", env.Endpoint,
	))
	// TODO: send invalid envelope to a quarantine endpoint when it exists.
}
