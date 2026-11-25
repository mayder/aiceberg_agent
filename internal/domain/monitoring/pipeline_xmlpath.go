package monitoring

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxPipelineXMLBytes      = 4096
	maxPipelineXMLDepth      = 16
	maxPipelineXMLTokens     = 512
	maxPipelineXMLAttributes = 32
)

const pipelineXMLNamePattern = `[A-Za-z_][A-Za-z0-9_.-]{0,63}`

var pipelineXMLName = regexp.MustCompile(`^` + pipelineXMLNamePattern + `$`)
var pipelineXMLSegment = regexp.MustCompile(`^(` + pipelineXMLNamePattern + `)(?:\[([1-9][0-9]{0,2})\])?$`)

type pipelineXMLSegmentSpec struct {
	name     string
	position int // Zero requires a unique child of this name.
}

type pipelineXMLPath struct {
	elements  []pipelineXMLSegmentSpec
	attribute string
}

func compilePipelineXMLPath(step PipelineStep) (compiledPipelineStep, error) {
	invalid := errors.New("invalid_pipeline_step")
	var path string
	if step.InputType != "text" || !definitionEnum(step.OutputType, "text", "number", "integer", "boolean") ||
		json.Unmarshal(step.Argument, &path) != nil || len(path) < 2 || len(path) > 256 || path[0] != '/' {
		return compiledPipelineStep{}, invalid
	}
	pathSpec, ok := parsePipelineXMLPath(path)
	if !ok {
		return compiledPipelineStep{}, invalid
	}
	return compiledPipelineStep{xmlPath: pathSpec}, nil
}

func parsePipelineXMLPath(path string) (pipelineXMLPath, bool) {
	parts := strings.Split(path[1:], "/")
	spec := pipelineXMLPath{}
	if strings.HasPrefix(parts[len(parts)-1], "@") {
		spec.attribute = parts[len(parts)-1][1:]
		if !pipelineXMLName.MatchString(spec.attribute) || spec.attribute == "xmlns" {
			return pipelineXMLPath{}, false
		}
		parts = parts[:len(parts)-1]
	}
	if len(parts) < 1 || len(parts) > maxPipelineXMLDepth {
		return pipelineXMLPath{}, false
	}
	for _, part := range parts {
		match := pipelineXMLSegment.FindStringSubmatch(part)
		if match == nil {
			return pipelineXMLPath{}, false
		}
		position, _ := strconv.Atoi(match[2])
		spec.elements = append(spec.elements, pipelineXMLSegmentSpec{name: match[1], position: position})
	}
	return spec, true
}

func pipelineXMLValue(path pipelineXMLPath, input, outputType string) (json.RawMessage, error) {
	value, err := selectPipelineXML(path, input)
	if err != nil {
		return nil, err
	}
	if outputType == "text" {
		if recordText(value, maxPipelineXMLBytes) {
			return json.Marshal(value)
		}
	} else {
		raw := json.RawMessage(strings.Trim(value, " \r\n\t"))
		if validRecordValue(outputType, raw) && (outputType != "integer" || pipelineIntegerLiteral(raw)) {
			return raw, nil
		}
	}
	return nil, errors.New("pipeline_value_invalid")
}
