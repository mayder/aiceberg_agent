package agentless

import (
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"strings"
)

// Additive metadata follows the observation through the existing durable outbox.
// Diagnostics/discovery do not become routine metric streams. No second probe.
func withMonitoringSamples(job entities.AgentlessJob, observation entities.AgentlessObservation) entities.AgentlessObservation {
	observation.MonitoringSamples = nil
	if strings.ToLower(job.Tipo) != "snmp" || job.CheckID != observation.CheckID {
		return observation
	}
	if marker, exists := job.Config["diagnostic_probe"]; exists && marker != false {
		return observation
	}
	freshness := max(int64(60), min(int64(604800), int64(job.IntervalSec)*3))
	authority := monitoring.DefinitionAuthority{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckIDs: []int64{int64(job.CheckID)}}
	definition, err := DescribeSNMPItems(job, authority, SNMPDefinitionPolicy{Revision: 1, FreshnessSec: freshness, HistoryDays: 1})
	if err != nil {
		return observation
	} // Metadata failure cannot discard the legacy collection.
	batch, err := monitoring.ProjectDefinedSNMP(definition, authority,
		monitoring.Observation{Status: observation.Status, Partial: observation.IsPartial, Final: observation.IsFinal, Payload: observation.Payload},
		observation.ObservedAt.UnixMilli(), observation.ObservedAt.UnixMilli())
	if err == nil {
		observation.MonitoringSamples = &batch
	}
	return observation
}
