package monitoring

import "encoding/json"

// Configuration is compiled data, never permission to schedule or execute a source.
type Configuration struct {
	Contract      string                   `json:"contract"`
	SchemaVersion int                      `json:"schema_version"`
	Definition    Definition               `json:"definition"`
	Root          ConfigurationReference   `json:"root"`
	Lineage       []ConfigurationReference `json:"lineage"`
	Items         []ConfigurationItem      `json:"items"`
}

type ConfigurationItem struct {
	TemplateKey      string                        `json:"template_key"`
	ItemKey          string                        `json:"item_key"`
	SourceItemID     int64                         `json:"source_item_id"`
	SourceTemplateID int64                         `json:"source_template_id"`
	SourceRevision   int64                         `json:"source_revision"`
	Parameters       map[string]ConfigurationValue `json:"parameters"`
}

type ConfigurationValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// Bindings, immutable pins and reference allowlists are resolved outside the body.
type ConfigurationAuthority struct {
	Revision           int64                           `json:"revision"`
	Root               ConfigurationReference          `json:"root"`
	Definition         DefinitionAuthority             `json:"definition"`
	Bindings           map[string]ConfigurationBinding `json:"bindings"`
	TemplateRevisions  map[int64]int64                 `json:"template_revisions"`
	SecretReferences   []string                        `json:"secret_references"`
	PipelineReferences []string                        `json:"pipeline_references"`
}

type ConfigurationBinding struct {
	CheckID          int64 `json:"check_id"`
	SourceItemID     int64 `json:"source_item_id"`
	SourceTemplateID int64 `json:"source_template_id"`
}

type ConfigurationReference struct {
	ID       int64 `json:"id"`
	Revision int64 `json:"revision"`
}
