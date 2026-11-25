package monitoring

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// CompilePipeline validates the exact pinned bytes, version, chain and all steps.
func CompilePipeline(raw []byte, a PipelineAuthority) (CompiledPipeline, error) {
	if err := validatePipelineAuthority(a); err != nil {
		return CompiledPipeline{}, err
	}
	if len(raw) == 0 || len(raw) > MaxPipelineBytes {
		return CompiledPipeline{}, errors.New("invalid_monitoring_pipeline")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != a.SHA256 {
		return CompiledPipeline{}, errors.New("pipeline_artifact_mismatch")
	}
	if !recordJSONValid(raw) || !pipelineShape(raw) {
		return CompiledPipeline{}, errors.New("invalid_monitoring_pipeline")
	}
	var p PipelineDefinition
	if json.Unmarshal(raw, &p) != nil {
		return CompiledPipeline{}, errors.New("invalid_monitoring_pipeline")
	}
	steps, err := compilePipelineSteps(p, a)
	if err != nil {
		return CompiledPipeline{}, err
	}
	a.ItemKeys = append([]string{}, a.ItemKeys...)
	return CompiledPipeline{definition: p, authority: a, steps: steps, stateful: pipelineHasState(p.Steps)}, nil
}

func validatePipelineAuthority(a PipelineAuthority) error {
	if !definitionInteger(a.ClientID, 1, 9007199254740991) || !definitionInteger(a.AssetID, 1, 9007199254740991) || !definitionInteger(a.ConfigurationRevision, 1, 9007199254740991) || !definitionInteger(a.Revision, 1, 9007199254740991) || !configurationIdentifier.MatchString(a.ID) || len(a.SHA256) != 64 || len(a.ItemKeys) < 1 || len(a.ItemKeys) > MaxDefinitionItems {
		return errors.New("invalid_pipeline_authority")
	}
	if _, err := hex.DecodeString(a.SHA256); err != nil || strings.ToLower(a.SHA256) != a.SHA256 {
		return errors.New("invalid_pipeline_authority")
	}
	prefix := fmt.Sprintf("v1:c:%d:a:%d:check:", a.ClientID, a.AssetID)
	seen := map[string]bool{}
	for _, key := range a.ItemKeys {
		if len(key) > 192 || !strings.HasPrefix(key, prefix) || seen[key] {
			return errors.New("invalid_pipeline_authority")
		}
		seen[key] = true
	}
	return nil
}

func pipelineShape(raw []byte) bool {
	object, ok := configurationObject(raw, "contract", "schema_version", "id", "revision", "client_id", "input_type", "input_unit", "output_type", "output_unit", "steps")
	if !ok {
		return false
	}
	var steps []map[string]json.RawMessage
	if json.Unmarshal(object["steps"], &steps) != nil || len(steps) < 1 || len(steps) > MaxPipelineSteps {
		return false
	}
	for _, s := range steps {
		if !definitionFields(s, "id", "operation", "input_type", "input_unit", "output_type", "output_unit", "argument") {
			return false
		}
		for name, value := range s {
			if name != "argument" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return false
			}
		}
	}
	return true
}

func compilePipelineSteps(p PipelineDefinition, a PipelineAuthority) ([]compiledPipelineStep, error) {
	if p.Contract != "monitoring.pipeline.v1" || p.SchemaVersion != 1 || p.ID != a.ID || p.Revision != a.Revision || p.ClientID != a.ClientID || len(p.Steps) < 1 || len(p.Steps) > MaxPipelineSteps || !pipelineType(p.InputType) || !pipelineType(p.OutputType) || !definitionUnit.MatchString(p.InputUnit) || !definitionUnit.MatchString(p.OutputUnit) {
		return nil, errors.New("invalid_monitoring_pipeline")
	}
	compiled := make([]compiledPipelineStep, 0, len(p.Steps))
	kind, unit := p.InputType, p.InputUnit
	seen := map[string]bool{}
	for index, step := range p.Steps {
		if (step.Operation == "throttle" && index != len(p.Steps)-1) || !configurationIdentifier.MatchString(step.ID) || seen[step.ID] || step.InputType != kind || step.InputUnit != unit || !pipelineType(step.OutputType) || !definitionUnit.MatchString(step.OutputUnit) {
			return nil, errors.New("invalid_pipeline_chain")
		}
		operation, err := compilePipelineStep(step)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, operation)
		seen[step.ID] = true
		kind, unit = step.OutputType, step.OutputUnit
	}
	if kind != p.OutputType || unit != p.OutputUnit {
		return nil, errors.New("invalid_pipeline_chain")
	}
	return compiled, nil
}

func validatePipelineStep(s PipelineStep) error {
	if s.Operation == "identity" {
		if s.InputType != s.OutputType || s.InputUnit != s.OutputUnit || !bytes.Equal(bytes.TrimSpace(s.Argument), []byte("null")) {
			return errors.New("invalid_pipeline_step")
		}
		return nil
	}
	if !definitionEnum(s.Operation, "scale", "offset") {
		return errors.New("unsupported_pipeline_step")
	}
	if !definitionEnum(s.InputType, "number", "integer") || !definitionEnum(s.OutputType, "number", "integer") || !validRecordValue("number", s.Argument) {
		return errors.New("invalid_pipeline_step")
	}
	return nil
}

func pipelineType(s string) bool {
	return definitionEnum(s, "number", "integer", "boolean", "text", "event")
}

func pipelineHasState(steps []PipelineStep) bool {
	for _, step := range steps {
		if definitionEnum(step.Operation, "delta", "throttle") {
			return true
		}
	}
	return false
}
