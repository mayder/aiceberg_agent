package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"

	"github.com/you/aiceberg_agent/internal/domain/monitoring/internal/scriptsandbox"
)

type compiledPipelineStep struct {
	operand  float64
	minimum  float64
	maximum  float64
	pattern  *regexp.Regexp
	group    int
	path     []pipelineSelector
	xmlPath  pipelineXMLPath
	counter  bool
	interval int64
	script   scriptsandbox.Program
}

func compilePipelineStep(step PipelineStep) (compiledPipelineStep, error) {
	switch step.Operation {
	case "script":
		return compilePipelineScript(step)
	case "delta", "throttle":
		return compilePipelineStateStep(step)
	case "validate":
		return compilePipelineValidation(step)
	case "regex":
		return compilePipelineRegex(step)
	case "jsonpath":
		return compilePipelinePath(step)
	case "xmlpath":
		return compilePipelineXMLPath(step)
	}
	if err := validatePipelineStep(step); err != nil {
		return compiledPipelineStep{}, err
	}
	if step.Operation == "identity" {
		return compiledPipelineStep{}, nil
	}
	return compiledPipelineStep{operand: pipelineNumber(step.Argument)}, nil
}

func compilePipelineValidation(step PipelineStep) (compiledPipelineStep, error) {
	invalid := errors.New("invalid_pipeline_step")
	if step.InputType != step.OutputType || step.InputUnit != step.OutputUnit ||
		!definitionEnum(step.InputType, "number", "integer", "text") {
		return compiledPipelineStep{}, invalid
	}
	args, ok := configurationObject(step.Argument, "min", "max")
	kind := step.InputType
	if kind == "text" {
		kind = "integer"
	}
	if !ok || !validRecordValue(kind, args["min"]) || !validRecordValue(kind, args["max"]) {
		return compiledPipelineStep{}, invalid
	}
	minimum, maximum := pipelineNumber(args["min"]), pipelineNumber(args["max"])
	if kind == "integer" && (!pipelineIntegerLiteral(args["min"]) || !pipelineIntegerLiteral(args["max"])) {
		return compiledPipelineStep{}, invalid
	}
	if minimum > maximum || (step.InputType == "text" && (minimum < 0 || maximum > 4096)) {
		return compiledPipelineStep{}, invalid
	}
	return compiledPipelineStep{minimum: minimum, maximum: maximum}, nil
}

func pipelineNumber(raw json.RawMessage) float64 {
	var value float64
	_ = json.Unmarshal(raw, &value) // The contract validated this finite number.
	return value
}

func pipelinePreprocess(step PipelineStep, compiled compiledPipelineStep, raw json.RawMessage) (json.RawMessage, error) {
	var input string
	if step.InputType == "text" {
		_ = json.Unmarshal(raw, &input)
	}
	switch step.Operation {
	case "validate":
		value := float64(len(input))
		if step.InputType != "text" {
			value = pipelineNumber(raw)
		}
		if value < compiled.minimum || value > compiled.maximum ||
			(step.InputType == "integer" && !pipelineIntegerLiteral(raw)) {
			return nil, errors.New("pipeline_validation_failed")
		}
		return append(json.RawMessage(nil), raw...), nil
	case "regex":
		return pipelineRegexExtract(compiled, input)
	case "xmlpath":
		return pipelineXMLValue(compiled.xmlPath, input, step.OutputType)
	default:
		return pipelineSelect(compiled.path, input, step.OutputType)
	}
}

// Extracted integers must be JSON integer literals. Never round a decimal into one.
func pipelineIntegerLiteral(raw json.RawMessage) bool {
	value, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	return err == nil && definitionInteger(value, -9007199254740991, 9007199254740991)
}
