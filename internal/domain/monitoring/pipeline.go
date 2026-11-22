package monitoring

import "encoding/json"

const MaxPipelineSteps = 32
const MaxPipelineBytes = 32768

// PipelineDefinition contains bounded built-in transformations, never code or IO.
type PipelineDefinition struct {
	Contract      string         `json:"contract"`
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Revision      int64          `json:"revision"`
	ClientID      int64          `json:"client_id"`
	InputType     string         `json:"input_type"`
	InputUnit     string         `json:"input_unit"`
	OutputType    string         `json:"output_type"`
	OutputUnit    string         `json:"output_unit"`
	Steps         []PipelineStep `json:"steps"`
}

type PipelineStep struct {
	ID         string          `json:"id"`
	Operation  string          `json:"operation"`
	InputType  string          `json:"input_type"`
	InputUnit  string          `json:"input_unit"`
	OutputType string          `json:"output_type"`
	OutputUnit string          `json:"output_unit"`
	Argument   json.RawMessage `json:"argument"`
}

// PipelineAuthority is pinned by the authenticated control, separately from the artifact.
type PipelineAuthority struct {
	ClientID              int64    `json:"client_id"`
	AssetID               int64    `json:"asset_id"`
	ConfigurationRevision int64    `json:"configuration_revision"`
	ID                    string   `json:"id"`
	Revision              int64    `json:"revision"`
	SHA256                string   `json:"sha256"`
	ItemKeys              []string `json:"item_keys"`
}

// CompiledPipeline owns its immutable artifact and authority, not caller slices.
type CompiledPipeline struct {
	definition PipelineDefinition
	authority  PipelineAuthority
	operands   []float64
}

// PipelineBudget is trusted execution policy, never an artifact-supplied allowance.
type PipelineBudget struct {
	MaxSteps       int `json:"max_steps"`
	MaxOperations  int `json:"max_operations"`
	MaxInputBytes  int `json:"max_input_bytes"`
	MaxOutputBytes int `json:"max_output_bytes"`
	TimeoutMS      int `json:"timeout_ms"`
}

type PipelineFailure struct {
	ObservedAtMS int64  `json:"observed_at_ms"`
	ItemKey      string `json:"item_key"`
	StepID       string `json:"step_id"`
	Code         string `json:"code"`
}

type PipelineExecution struct {
	Contract         string            `json:"contract"`
	SchemaVersion    int               `json:"schema_version"`
	PipelineSHA256   string            `json:"pipeline_sha256"`
	Batch            RecordsBatch      `json:"batch"`
	PipelineID       string            `json:"pipeline_id"`
	PipelineRevision int64             `json:"pipeline_revision"`
	Processed        int               `json:"processed"`
	Skipped          int               `json:"skipped"`
	Failed           int               `json:"failed"`
	Operations       int               `json:"operations"`
	Failures         []PipelineFailure `json:"failures"`
}
