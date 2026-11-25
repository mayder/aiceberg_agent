package monitoring

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math"
	"strconv"
)

var (
	errPipelineWarmup        = errors.New("pipeline_delta_warmup")
	errPipelineSuppressed    = errors.New("pipeline_throttle_suppressed")
	errPipelineStateRequired = errors.New("pipeline_state_required")
)

type pipelineStepState struct {
	Number float64  `json:"number,omitempty"`
	At     int64    `json:"at,omitempty"`
	Digest [32]byte `json:"digest,omitempty"`
}

func compilePipelineStateStep(step PipelineStep) (compiledPipelineStep, error) {
	invalid := errors.New("invalid_pipeline_step")
	if step.InputType != step.OutputType || step.InputUnit != step.OutputUnit {
		return compiledPipelineStep{}, invalid
	}
	if step.Operation == "delta" {
		var mode string
		if !definitionEnum(step.InputType, "number", "integer") || json.Unmarshal(step.Argument, &mode) != nil ||
			!definitionEnum(mode, "counter", "gauge") {
			return compiledPipelineStep{}, invalid
		}
		return compiledPipelineStep{counter: mode == "counter"}, nil
	}
	if !definitionEnum(step.InputType, "number", "integer", "text", "boolean") || !pipelineIntegerLiteral(step.Argument) {
		return compiledPipelineStep{}, invalid
	}
	interval := int64(pipelineNumber(step.Argument))
	if interval < 1 || interval > 86400000 {
		return compiledPipelineStep{}, invalid
	}
	return compiledPipelineStep{interval: interval}, nil
}

func pipelineDelta(step PipelineStep, cfg compiledPipelineStep, raw json.RawMessage, state *pipelineStepState, at int64) (json.RawMessage, error) {
	if !validRecordValue(step.InputType, raw) || (step.InputType == "integer" && !pipelineIntegerLiteral(raw)) {
		return nil, errors.New("pipeline_value_invalid")
	}
	current := pipelineNumber(raw)
	if cfg.counter && current < 0 {
		return nil, errors.New("pipeline_value_invalid")
	}
	prior := *state
	*state = pipelineStepState{Number: current, At: at}
	if prior.At == 0 || (cfg.counter && current < prior.Number) {
		return nil, errPipelineWarmup
	}
	if step.InputType == "integer" {
		value := int64(current) - int64(prior.Number)
		if !definitionInteger(value, -9007199254740991, 9007199254740991) {
			return nil, errors.New("pipeline_numeric_range")
		}
		return json.Marshal(value)
	}
	value := current - prior.Number
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return nil, errors.New("pipeline_numeric_range")
	}
	return json.Marshal(value)
}

func pipelineThrottle(step PipelineStep, cfg compiledPipelineStep, raw json.RawMessage, state *pipelineStepState, r TypedRecord) (json.RawMessage, error) {
	if !validRecordValue(step.InputType, raw) || (step.InputType == "integer" && !pipelineIntegerLiteral(raw)) {
		return nil, errors.New("pipeline_value_invalid")
	}
	digest := pipelineThrottleDigest(step.InputType, raw, r.State)
	if state.At != 0 && state.Digest == digest && r.ObservedAtMS-state.At < cfg.interval {
		return nil, errPipelineSuppressed
	}
	*state = pipelineStepState{At: r.ObservedAtMS, Digest: digest}
	return raw, nil
}

func pipelineThrottleDigest(kind string, raw json.RawMessage, quality string) [32]byte {
	var value string
	switch kind {
	case "number", "integer":
		number := pipelineNumber(raw)
		if number != 0 {
			value = strconv.FormatFloat(number, 'g', -1, 64)
		}
	case "text":
		_ = json.Unmarshal(raw, &value)
	case "boolean":
		var flag bool
		_ = json.Unmarshal(raw, &flag)
		value = strconv.FormatBool(flag)
	}
	return sha256.Sum256([]byte(quality + ":" + value))
}
