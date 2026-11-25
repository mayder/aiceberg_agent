package monitoring

import (
	"encoding/json"
	"errors"
)

const graphInputContract = "monitoring.graph-input.v1"

type graphInput struct {
	Contract      string       `json:"contract"`
	SchemaVersion int          `json:"schema_version"`
	GraphSHA256   string       `json:"graph_sha256"`
	Batch         RecordsBatch `json:"batch"`
}

// EncodeGraphInput stages a raw collection snapshot for the exact compiled graph.
// It rejects derived records and repeated samples of the same collected item.
func EncodeGraphInput(g CompiledGraph, batch RecordsBatch) ([]byte, error) {
	if err := graphValidateInput(g, batch, batch.EvaluatedAtMS); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(graphInput{graphInputContract, 1, g.digest, batch})
	if err != nil || len(raw) > MaxRecordBytes {
		return nil, errors.New("monitoring_graph_input_limit")
	}
	return raw, nil
}

func graphDecodeInput(g CompiledGraph, b PipelineBudget, raw []byte, now int64) (RecordsBatch, error) {
	invalid := errors.New("invalid_monitoring_graph_input")
	if len(raw) > b.MaxInputBytes {
		return RecordsBatch{}, errors.New("monitoring_graph_input_limit")
	}
	if g.digest == "" || !recordJSONValid(raw) {
		return RecordsBatch{}, invalid
	}
	fields, ok := configurationObject(raw, "contract", "schema_version", "graph_sha256", "batch")
	var header struct {
		Contract      string `json:"contract"`
		SchemaVersion int    `json:"schema_version"`
		GraphSHA256   string `json:"graph_sha256"`
	}
	if !ok || json.Unmarshal(raw, &header) != nil || header.Contract != graphInputContract ||
		header.SchemaVersion != 1 || header.GraphSHA256 != g.digest {
		return RecordsBatch{}, invalid
	}
	batch, err := DecodeRecords(fields["batch"])
	if err != nil {
		return RecordsBatch{}, err
	}
	return batch, graphValidateInput(g, batch, now)
}

func graphValidateInput(g CompiledGraph, batch RecordsBatch, now int64) error {
	if err := ValidateRecords(batch, g.input, g.authority, now); err != nil {
		return err
	}
	seen := make(map[string]bool, len(batch.Records))
	for _, record := range batch.Records {
		if seen[record.ItemKey] {
			return errors.New("monitoring_graph_duplicate_source")
		}
		seen[record.ItemKey] = true
	}
	return nil
}
