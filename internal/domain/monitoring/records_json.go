package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeRecords rejects ambiguous wire representations before semantic validation.
func DecodeRecords(raw []byte) (RecordsBatch, error) {
	fail := errors.New("invalid_monitoring_records")
	if len(raw) == 0 || len(raw) > MaxRecordBytes || !recordUnicode(raw) {
		return RecordsBatch{}, fail
	}
	if !recordJSONValid(raw) {
		return RecordsBatch{}, fail
	}
	if !recordBatchShape(raw) {
		return RecordsBatch{}, fail
	}
	var batch RecordsBatch
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&batch) != nil {
		return RecordsBatch{}, fail
	}
	return batch, nil
}

func recordBatchShape(raw []byte) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || !definitionFields(object, "contract", "schema_version", "definition_revision", "client_id", "asset_id", "coverage", "evaluated_at_ms", "records") {
		return false
	}
	for _, value := range object {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	var records []map[string]json.RawMessage
	if json.Unmarshal(object["records"], &records) != nil || len(records) < 1 || len(records) > MaxRecords {
		return false
	}
	for _, record := range records {
		if !definitionFields(record, "item_key", "value_type", "unit", "observed_at_ms", "state", "reason", "value") {
			return false
		}
		for key, value := range record {
			if key != "value" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return false
			}
		}
	}
	return true
}

// EncodeRecords validates the complete contract; it does not enqueue or persist.
func EncodeRecords(batch RecordsBatch, d Definition, a DefinitionAuthority, now int64) ([]byte, error) {
	if err := ValidateRecords(batch, d, a, now); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(batch); err != nil {
		return nil, errors.New("invalid_monitoring_records")
	}
	raw := bytes.TrimSuffix(out.Bytes(), []byte("\n"))
	if len(raw) > MaxRecordBytes {
		return nil, errors.New("invalid_monitoring_records")
	}
	return raw, nil
}

func recordJSONValid(raw []byte) bool {
	if !recordUnicode(raw) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if definitionJSONValue(decoder, 0) != nil {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
