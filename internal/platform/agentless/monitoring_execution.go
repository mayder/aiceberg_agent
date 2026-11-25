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
	if (kind != "snmp" && !monitoring.IsNumericProbe(kind) && !monitoring.LatencySource(kind)) || job.CheckID != observation.CheckID {
		return observation
	}
	if marker, exists := job.Config["diagnostic_probe"]; exists && marker != false {
		return observation
	}
	freshness := jobProbeFreshness(job)
	if monitoring.IsNumericProbe(kind) {
		return withNumericProbeSamples(kind, job, observation, freshness)
	}
	authority := monitoring.DefinitionAuthority{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckIDs: []int64{int64(job.CheckID)}}
	if monitoring.LatencySource(kind) {
		return withLatencyMonitoringSamples(kind, job, observation, authority, freshness)
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

func withNumericProbeSamples(kind string, job entities.AgentlessJob, observation entities.AgentlessObservation, freshness int64) entities.AgentlessObservation {
	scope := monitoring.Scope{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckID: int64(job.CheckID)}
	batch, err := monitoring.ProjectConfiguredNumericProbe(scope, kind, job.Config,
		monitoring.Observation{Status: observation.Status, Partial: observation.IsPartial, Final: !observation.IsPartial, Payload: observation.Payload},
		observation.ObservedAt.UnixMilli(), freshness)
	if err == nil {
		observation.MonitoringSamples = &batch
	}
	return observation
}

// Non-segmented latency observations predate IsFinal. Their legacy zero segment
// metadata means a finished attempt; explicit partial/segmented states stay strict.
func withLatencyMonitoringSamples(source string, job entities.AgentlessJob, observation entities.AgentlessObservation, authority monitoring.DefinitionAuthority, freshness int64) entities.AgentlessObservation {
	definition := monitoring.LatencyCompatibilityDefinition(monitoring.Scope{ClientID: authority.ClientID, AssetID: authority.AssetID, CheckID: int64(job.CheckID)}, source, freshness)
	final := observation.IsFinal || (!observation.IsPartial && observation.SegmentSeq == 0)
	batch, err := monitoring.ProjectDefinedLatency(definition, authority, source, monitoring.Observation{Status: observation.Status, Partial: observation.IsPartial, Final: final, Payload: observation.Payload}, observation.LatencyMs, observation.ObservedAt.UnixMilli(), observation.ObservedAt.UnixMilli())
	if err == nil {
		observation.MonitoringSamples = &batch
	}
	return observation
}

func jobProbeFreshness(job entities.AgentlessJob) int64 {
	interval := job.BaseIntervalSec
	if interval <= 0 {
		interval = job.IntervalSec
	}
	return monitoring.ProbeFreshness(int64(interval))
}
