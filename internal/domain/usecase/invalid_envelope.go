package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// HandleInvalidEnvelope centralizes handling for envelopes we refuse to send.
func HandleInvalidEnvelope(log logger.Logger, env entities.Envelope, reason string) {
	if log == nil {
		return
	}
	msg := "invalid envelope dropped reason=" + reason
	if env.ID != "" {
		msg += " envelope_id=" + env.ID
	}
	if env.AgentID != "" {
		msg += " agent_id=" + env.AgentID
	}
	if env.Endpoint != "" {
		msg += " endpoint=" + env.Endpoint
	}
	log.Error(msg)
	// TODO: send invalid envelope to a quarantine endpoint when it exists.
}
