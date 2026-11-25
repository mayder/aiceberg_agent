package monitoring

import (
	"encoding/json"
	"errors"
)

var errGraphProjection = errors.New("monitoring_graph_projection_mismatch")

// EncodeProjectedGraphInput consumes server-side projections from an
// independently authorized check. Raw agent-supplied projection metadata
// must not be passed here in place of a projection from an authorized job.
func EncodeProjectedGraphInput(g CompiledGraph, projections []Batch, evaluated int64) ([]byte, error) {
	if len(projections) < 1 || len(projections) > MaxRecords {
		return nil, errGraphProjection
	}
	batch := RecordsBatch{Contract: "monitoring.records.v1", SchemaVersion: 1, DefinitionRevision: g.input.Revision,
		ClientID: g.input.ClientID, AssetID: g.input.AssetID, EvaluatedAtMS: evaluated, Coverage: "partial"}
	items := make(map[string]ItemDefinition, len(g.input.Items))
	for _, item := range g.input.Items {
		items[item.Key] = item
	}
	for _, projection := range projections {
		if projection.SchemaVersion != 1 || projection.Scope.ClientID != batch.ClientID || projection.Scope.AssetID != batch.AssetID ||
			len(projection.Samples) == 0 || len(projection.Samples) > MaxRecords-len(batch.Records) {
			return nil, errGraphProjection
		}
		for _, sample := range projection.Samples {
			item, exists := items[sample.ItemKey]
			if !exists {
				return nil, errGraphProjection
			}
			r, err := projectGraphRecord(projection, sample, item, evaluated)
			if err != nil {
				return nil, err
			}
			batch.Records = append(batch.Records, r)
		}
	}
	if len(batch.Records) == len(items) {
		batch.Coverage = "complete"
	}
	return EncodeGraphInput(g, batch)
}

func projectGraphRecord(b Batch, s Sample, item ItemDefinition, evaluated int64) (TypedRecord, error) {
	if (!LatencySource(item.Source) && item.Source != "snmp" && !IsNumericProbe(item.Source)) || b.Source != "agentless_"+item.Source ||
		item.CheckID != b.Scope.CheckID || item.Metric != s.Metric || item.Unit != s.Unit ||
		!definitionEnum(item.ValueType, "number", "integer") {
		return TypedRecord{}, errGraphProjection
	}
	r := TypedRecord{ItemKey: item.Key, ValueType: item.ValueType, Unit: item.Unit, ObservedAtMS: b.ObservedAtMS,
		State: s.State, Reason: s.Gap, Value: json.RawMessage("null")}
	if definitionEnum(s.State, "ok", "problem") {
		if s.Value == nil || s.Gap != "" {
			return TypedRecord{}, errGraphProjection
		}
		raw, err := json.Marshal(*s.Value)
		if err != nil || !validRecordValue(item.ValueType, raw) {
			return TypedRecord{}, errGraphProjection
		}
		r.Value, r.Reason = raw, "none"
	} else if s.Value != nil || s.State != "unknown" || !definitionEnum(s.Gap, "future", "stale", "missing", "invalid", "partial", "time_budget", "observation_failed") {
		return TypedRecord{}, errGraphProjection
	}
	freshness, err := RecordFreshness(r.ObservedAtMS, item.FreshnessSec, evaluated)
	if err != nil {
		return TypedRecord{}, err
	}
	if freshness == "future" {
		r.State, r.Reason, r.Value = "unknown", "future", json.RawMessage("null")
	} else if freshness == "stale" {
		r.State, r.Reason, r.Value = "stale", "stale", json.RawMessage("null")
	} else if r.Reason == "stale" || r.Reason == "future" {
		r.Reason = "observation_failed"
	}
	return r, nil
}
