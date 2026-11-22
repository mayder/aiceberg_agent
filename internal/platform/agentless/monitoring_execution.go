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
	kind := strings.ToLower(job.Tipo)
	if (kind != "snmp" && kind != "tcp") || job.CheckID != observation.CheckID {
		return observation
	}
	if marker, exists := job.Config["diagnostic_probe"]; exists && marker != false {
		return observation
	}
	freshness := max(int64(60), min(int64(604800), int64(job.IntervalSec)*3))
	authority := monitoring.DefinitionAuthority{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckIDs: []int64{int64(job.CheckID)}}
	if kind == "tcp" {
		return withTCPMonitoringSamples(job, observation, authority, freshness)
	}
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

// Non-segmented TCP observations predate IsFinal. Their legacy zero segment
// metadata means a finished attempt; explicit partial/segmented states stay strict.
func withTCPMonitoringSamples(job entities.AgentlessJob, observation entities.AgentlessObservation, authority monitoring.DefinitionAuthority, freshness int64) entities.AgentlessObservation {
	definition := monitoring.TCPCompatibilityDefinition(monitoring.Scope{ClientID: authority.ClientID, AssetID: authority.AssetID, CheckID: int64(job.CheckID)}, freshness)
	final := observation.IsFinal || (!observation.IsPartial && observation.SegmentSeq == 0)
	batch, err := monitoring.ProjectDefinedTCP(definition, authority, monitoring.Observation{Status: observation.Status, Partial: observation.IsPartial, Final: final, Payload: observation.Payload}, observation.LatencyMs, observation.ObservedAt.UnixMilli(), observation.ObservedAt.UnixMilli())
	if err == nil {
		observation.MonitoringSamples = &batch
	}
	return observation
}
