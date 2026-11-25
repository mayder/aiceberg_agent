package monitoring

import (
	"bytes"
	"errors"
	"strconv"
)

func ValidateRecords(batch RecordsBatch, d Definition, a DefinitionAuthority, now int64) error {
	if err := validateRecordsEnvelope(batch, d, a, now); err != nil {
		return err
	}
	items := map[string]ItemDefinition{}
	for _, item := range d.Items {
		if item.Enabled {
			items[item.Key] = item
		}
	}
	seen := map[string]bool{}
	coverage := map[string]bool{}
	for _, record := range batch.Records {
		identity, err := validateTypedRecord(record, items, batch.EvaluatedAtMS)
		if err != nil {
			return err
		}
		if seen[identity] {
			return errors.New("duplicate_monitoring_record")
		}
		seen[identity] = true
		coverage[record.ItemKey] = true
	}
	if batch.Coverage == "complete" && len(coverage) != len(items) {
		return errors.New("incomplete_monitoring_records")
	}
	return nil
}

func validateTypedRecord(record TypedRecord, items map[string]ItemDefinition, now int64) (string, error) {
	item, ok := items[record.ItemKey]
	if !ok {
		return "", errors.New("monitoring_record_scope_mismatch")
	}
	if record.ValueType != item.ValueType || record.Unit != item.Unit {
		return "", errors.New("monitoring_record_type_mismatch")
	}
	if !definitionInteger(record.ObservedAtMS, 1, 253402300799999) {
		return "", errors.New("invalid_monitoring_clock")
	}
	if err := validateRecordState(record, now, item.FreshnessSec); err != nil {
		return "", err
	}
	if definitionEnum(record.State, "ok", "problem") && !validRecordValue(record.ValueType, record.Value) {
		return "", errors.New("invalid_monitoring_record_value")
	}
	return seriesRecordIdentity(record), nil
}

func seriesRecordIdentity(record TypedRecord) string {
	suffix := "time:" + strconv.FormatInt(record.ObservedAtMS, 10)
	if record.ValueType == "event" && !bytes.Equal(bytes.TrimSpace(record.Value), []byte("null")) {
		event, _ := decodeRecordEvent(record.Value)
		suffix = "event:" + event.EventID
	}
	return record.ItemKey + "|" + suffix
}

func validateRecordState(record TypedRecord, now, freshness int64) error {
	if !validRecordStateReason(record.State, record.Reason) {
		return errors.New("invalid_monitoring_record_state")
	}
	future := record.ObservedAtMS > now
	stale := now-record.ObservedAtMS > freshness*1000
	if (future && (record.State != "unknown" || record.Reason != "future")) || (!future && record.Reason == "future") || (stale && record.State != "stale") || (!stale && record.State == "stale") {
		return errors.New("invalid_monitoring_record_state")
	}
	if !definitionEnum(record.State, "ok", "problem") && !bytes.Equal(bytes.TrimSpace(record.Value), []byte("null")) {
		return errors.New("invalid_monitoring_record_value")
	}
	return nil
}

func validRecordStateReason(state, reason string) bool {
	reasons := map[string][]string{"ok": {"none"}, "problem": {"none"}, "unknown": {"missing", "invalid", "observation_failed", "partial", "time_budget", "future", "dependency_unavailable"}, "stale": {"stale"}, "unsupported": {"unsupported"}, "insufficient_data": {"insufficient_data"}}
	return definitionEnum(reason, reasons[state]...)
}

func validateRecordsEnvelope(batch RecordsBatch, d Definition, a DefinitionAuthority, now int64) error {
	if err := ValidateDefinition(d, a); err != nil {
		return err
	}
	if !definitionInteger(now, 1, 253402300799999) {
		return errors.New("invalid_monitoring_clock")
	}
	if batch.Contract != "monitoring.records.v1" || batch.SchemaVersion != 1 || !definitionInteger(batch.DefinitionRevision, 1, 9007199254740991) ||
		!definitionEnum(batch.Coverage, "partial", "complete") || len(batch.Records) < 1 || len(batch.Records) > MaxRecords {
		return errors.New("invalid_monitoring_records")
	}
	if !definitionInteger(batch.EvaluatedAtMS, 1, 253402300799999) || batch.EvaluatedAtMS > now {
		return errors.New("invalid_monitoring_clock")
	}
	if batch.ClientID != a.ClientID || batch.AssetID != a.AssetID {
		return errors.New("monitoring_record_scope_mismatch")
	}
	if batch.DefinitionRevision != d.Revision {
		return errors.New("monitoring_record_revision_mismatch")
	}
	return nil
}

// RecordFreshness never rewrites a historical record or infers health from age.
func RecordFreshness(observed, freshness, now int64) (string, error) {
	if !definitionInteger(observed, 1, 253402300799999) || !definitionInteger(now, 1, 253402300799999) || !definitionInteger(freshness, 1, 604800) {
		return "", errors.New("invalid_monitoring_clock")
	}
	if observed > now {
		return "future", nil
	}
	if now-observed > freshness*1000 {
		return "stale", nil
	}
	return "fresh", nil
}
