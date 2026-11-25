package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
)

type pipelineSelector struct {
	name  string
	index int // -1 selects an object member.
}

var pipelinePathSegment = regexp.MustCompile(`^(?:\.([A-Za-z_][A-Za-z0-9_]*)|\[(?:(0|[1-9][0-9]{0,3})|("(?:[^"\\]|\\.)*"))\])`)

func compilePipelinePath(step PipelineStep) (compiledPipelineStep, error) {
	invalid := errors.New("invalid_pipeline_step")
	var path string
	if step.InputType != "text" || json.Unmarshal(step.Argument, &path) != nil ||
		len(path) == 0 || path[0] != '$' || !recordText(path, 256) {
		return compiledPipelineStep{}, invalid
	}
	selectors := make([]pipelineSelector, 0, 4)
	for rest := path[1:]; rest != ""; {
		match := pipelinePathSegment.FindStringSubmatch(rest)
		if match == nil || len(selectors) == 16 {
			return compiledPipelineStep{}, invalid
		}
		selector, ok := decodePipelineSelector(match)
		if !ok {
			return compiledPipelineStep{}, invalid
		}
		selectors = append(selectors, selector)
		rest = rest[len(match[0]):]
	}
	return compiledPipelineStep{path: selectors}, nil
}

func decodePipelineSelector(match []string) (pipelineSelector, bool) {
	selector := pipelineSelector{name: match[1], index: -1}
	if match[2] != "" {
		selector.index, _ = strconv.Atoi(match[2])
	} else if match[3] != "" {
		raw := []byte(match[3])
		return selectorWithName(selector, raw)
	}
	return selector, true
}

func selectorWithName(selector pipelineSelector, raw []byte) (pipelineSelector, bool) {
	if !recordJSONValid(raw) || json.Unmarshal(raw, &selector.name) != nil {
		return pipelineSelector{}, false
	}
	return selector, recordText(selector.name, 256)
}

func pipelineSelect(path []pipelineSelector, document string, outputType string) (json.RawMessage, error) {
	current := json.RawMessage(document)
	if !recordJSONValid(current) {
		return nil, errors.New("pipeline_json_invalid")
	}
	for _, selector := range path {
		current = pipelineSelectChild(current, selector)
		if current == nil {
			return nil, errors.New("pipeline_jsonpath_missing")
		}
	}
	current = bytes.TrimSpace(current)
	if !validRecordValue(outputType, current) || (outputType == "integer" && !pipelineIntegerLiteral(current)) {
		return nil, errors.New("pipeline_value_invalid")
	}
	return current, nil
}

func pipelineSelectChild(raw json.RawMessage, selector pipelineSelector) json.RawMessage {
	if selector.index < 0 {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) == nil {
			return object[selector.name]
		}
	} else {
		var array []json.RawMessage
		if json.Unmarshal(raw, &array) == nil && selector.index < len(array) {
			return array[selector.index]
		}
	}
	return nil
}
