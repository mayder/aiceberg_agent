package mappers

import (
	"github.com/you/aiceberg_agent/internal/data/models"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func ToDTO(e entities.Envelope) models.EnvelopeDTO {
	return models.EnvelopeDTO{
		ID:            e.ID,
		Kind:          e.Kind,
		SchemaVersion: e.SchemaVersion,
		TSUnixMs:      e.TSUnixMs,
		Body:          e.Body,
		TenantID:      e.TenantID,
		AgentID:       e.AgentID,
		Meta:          e.Meta,
	}
}
