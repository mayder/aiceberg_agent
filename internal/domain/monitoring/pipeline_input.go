package monitoring

import (
	"encoding/json"
	"errors"
)

// PipelineInput is a local, explicitly unprocessed stage, distinct from final records/results.
type PipelineInput struct {
	Contract         string       `json:"contract"`
	SchemaVersion    int          `json:"schema_version"`
	PipelineID       string       `json:"pipeline_id"`
	PipelineRevision int64        `json:"pipeline_revision"`
	PipelineSHA256   string       `json:"pipeline_sha256"`
	Batch            RecordsBatch `json:"batch"`
}

func EncodePipelineInput(batch RecordsBatch, a PipelineAuthority) ([]byte, error) {
	if err := validatePipelineAuthority(a); err != nil {
		return nil, err
	}
	input := PipelineInput{Contract: "monitoring.pipeline-input.v1", SchemaVersion: 1, PipelineID: a.ID, PipelineRevision: a.Revision, PipelineSHA256: a.SHA256, Batch: batch}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > MaxRecordBytes {
		return nil, errors.New("invalid_pipeline_input")
	}
	return raw, nil
}

func decodePipelineInput(raw []byte, p CompiledPipeline) (RecordsBatch, error) {
	fail := errors.New("invalid_pipeline_input")
	if len(raw) == 0 || len(raw) > MaxRecordBytes || !recordJSONValid(raw) {
		return RecordsBatch{}, fail
	}
	object, ok := configurationObject(raw, "contract", "schema_version", "pipeline_id", "pipeline_revision", "pipeline_sha256", "batch")
	if !ok {
		return RecordsBatch{}, fail
	}
	var header struct {
		Contract      string `json:"contract"`
		SchemaVersion int    `json:"schema_version"`
		ID            string `json:"pipeline_id"`
		Revision      int64  `json:"pipeline_revision"`
		SHA256        string `json:"pipeline_sha256"`
	}
	if json.Unmarshal(raw, &header) != nil || header.Contract != "monitoring.pipeline-input.v1" || header.SchemaVersion != 1 || header.ID != p.authority.ID || header.Revision != p.authority.Revision || header.SHA256 != p.authority.SHA256 {
		return RecordsBatch{}, fail
	}
	return DecodeRecords(object["batch"])
}
