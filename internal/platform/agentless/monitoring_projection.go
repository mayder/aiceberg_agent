package agentless

import (
	"errors"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

// ProjectSNMPObservation derives samples from one finished collection. It does not
// run another probe or change the legacy observation, outbox or alarm ownership.
// Canonical transport activation remains a separate server compatibility gate.
func ProjectSNMPObservation(job entities.AgentlessJob, observation entities.AgentlessObservation, now time.Time, freshnessSec int64) (monitoring.Batch, error) {
	if strings.ToLower(job.Tipo) != "snmp" || job.CheckID != observation.CheckID {
		return monitoring.Batch{}, errors.New("invalid_monitoring_observation")
	}
	return monitoring.ProjectSNMP(
		monitoring.Scope{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckID: int64(job.CheckID)},
		monitoring.Observation{Status: observation.Status, Partial: observation.IsPartial, Final: observation.IsFinal, Payload: observation.Payload},
		observation.ObservedAt.UnixMilli(), now.UnixMilli(), freshnessSec,
	)
}
