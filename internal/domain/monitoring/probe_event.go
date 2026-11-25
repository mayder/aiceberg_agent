package monitoring

import (
	"encoding/json"
	"fmt"
)

// ProbeEventEvidence belongs to a persisted observation selected by an
// authorized reader. Report is nil for observations without a trusted receipt.
type ProbeEventEvidence struct {
	ObservationID int64
	Report        *ProbeReport
}

// ObservedGraphSource combines a trusted projection with its persisted receipt.
type ObservedGraphSource struct {
	Batch Batch
	Event ProbeEventEvidence
}

// EncodeObservedGraphInput projects one persisted collector observation onto
// the active input items. Numeric and event items share the same collection.
func EncodeObservedGraphInput(g CompiledGraph, measured Batch, event ProbeEventEvidence) ([]byte, error) {
	return EncodeObservedGraphSnapshot(g, []ObservedGraphSource{{Batch: measured, Event: event}})
}

// EncodeObservedGraphSnapshot joins the approved projections for one evaluation
// instant. It never fetches a missing observation or chooses between duplicates.
func EncodeObservedGraphSnapshot(g CompiledGraph, sources []ObservedGraphSource) ([]byte, error) {
	if len(sources) == 0 || len(sources) > MaxRecords {
		return nil, errGraphProjection
	}
	at := sources[0].Batch.ObservedAtMS
	input := RecordsBatch{Contract: "monitoring.records.v1", SchemaVersion: 1, DefinitionRevision: g.input.Revision,
		ClientID: g.input.ClientID, AssetID: g.input.AssetID, EvaluatedAtMS: at, Coverage: "complete"}
	byCheck := make(map[int64]ObservedGraphSource, len(sources))
	for _, source := range sources {
		batch := source.Batch
		if batch.SchemaVersion != 1 || batch.Scope.ClientID != input.ClientID || batch.Scope.AssetID != input.AssetID ||
			batch.ObservedAtMS != at || batch.Scope.CheckID < 1 || len(batch.Samples) > MaxRecords {
			return nil, errGraphProjection
		}
		if _, repeated := byCheck[batch.Scope.CheckID]; repeated {
			return nil, errGraphProjection
		}
		byCheck[batch.Scope.CheckID] = source
	}
	records, err := observedGraphRecords(g.input.Items, sources, byCheck, at)
	if err != nil {
		return nil, err
	}
	input.Records = records
	return EncodeGraphInput(g, input)
}

func observedGraphRecords(items []ItemDefinition, ordered []ObservedGraphSource, sources map[int64]ObservedGraphSource, at int64) ([]TypedRecord, error) {
	selected := make(map[int64]bool, len(sources))
	numericItems := make(map[string]ItemDefinition, len(items))
	eventItems := make([]ItemDefinition, 0, len(items))
	for _, item := range items {
		source, found := sources[item.CheckID]
		if !found || source.Batch.Source != "agentless_"+item.Source {
			return nil, errGraphProjection
		}
		selected[item.CheckID] = true
		if item.ValueType == "event" && item.Unit == "none" && item.Metric == "probe_outcome" {
			eventItems = append(eventItems, item)
		} else {
			numericItems[item.Key] = item
		}
	}
	if len(selected) != len(sources) {
		return nil, errGraphProjection
	}
	records, err := observedNumericRecords(ordered, numericItems, at)
	if err != nil {
		return nil, err
	}
	for _, item := range eventItems {
		source := sources[item.CheckID]
		record, err := projectProbeEvent(item, source.Batch.Scope, at, source.Event)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func observedNumericRecords(sources []ObservedGraphSource, items map[string]ItemDefinition, at int64) ([]TypedRecord, error) {
	records := make([]TypedRecord, 0, len(items))
	for _, source := range sources {
		for _, sample := range source.Batch.Samples {
			item, exists := items[sample.ItemKey]
			if !exists {
				continue
			}
			record, err := projectGraphRecord(source.Batch, sample, item, at)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	if len(records) != len(items) {
		return nil, errGraphProjection
	}
	return records, nil
}

func projectProbeEvent(item ItemDefinition, scope Scope, observed int64, evidence ProbeEventEvidence) (TypedRecord, error) {
	value := TypedRecord{ItemKey: item.Key, ValueType: "event", Unit: "none", ObservedAtMS: observed,
		State: "unknown", Reason: "missing", Value: json.RawMessage("null")}
	p := evidence.Report
	if p == nil {
		return value, nil
	}
	if evidence.ObservationID < 1 || p.Validate(scope, item.Source) != nil || p.ObservedAtMS/1000 != observed/1000 {
		return TypedRecord{}, errGraphProjection
	}
	if p.Outcome == "partial" {
		value.Reason = "partial"
		return value, nil
	}
	wire, err := json.Marshal(RecordEvent{
		EventID: fmt.Sprintf("probe:%d:%d:%d", scope.ClientID, scope.CheckID, evidence.ObservationID),
		Code:    "probe." + p.Outcome, Message: p.Reason,
	})
	if err != nil {
		return TypedRecord{}, err
	}
	value.State, value.Reason, value.Value = "ok", "none", wire
	return value, nil
}
