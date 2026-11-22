package monitoring

import "encoding/json"

const MaxRecordBytes = 262144
const MaxRecords = 128

// RecordsBatch is inert typed telemetry, distinct from active SNMP/TCP metadata.
// Policy, definitions and evaluation time are supplied by an authorized caller.
type RecordsBatch struct {
	Contract           string        `json:"contract"`
	SchemaVersion      int           `json:"schema_version"`
	DefinitionRevision int64         `json:"definition_revision"`
	ClientID           int64         `json:"client_id"`
	AssetID            int64         `json:"asset_id"`
	Coverage           string        `json:"coverage"`
	EvaluatedAtMS      int64         `json:"evaluated_at_ms"`
	Records            []TypedRecord `json:"records"`
}

type TypedRecord struct {
	ItemKey      string          `json:"item_key"`
	ValueType    string          `json:"value_type"`
	Unit         string          `json:"unit"`
	ObservedAtMS int64           `json:"observed_at_ms"`
	State        string          `json:"state"`
	Reason       string          `json:"reason"`
	Value        json.RawMessage `json:"value"`
}

type RecordEvent struct {
	EventID string `json:"event_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
