package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
)

const (
	MaxPipelineCheckpointBytes = 4 * 1024 * 1024
	pipelineReplayWindow       = 8
)

type pipelineItemState struct {
	ObservedAtMS int64                     `json:"observed_at_ms"`
	Steps        map[int]pipelineStepState `json:"steps"`
}

type pipelineReceipt struct {
	InputDigest string          `json:"input_sha256"`
	Result      json.RawMessage `json:"result"`
}

type pipelineCheckpoint struct {
	EvaluationMS int64                         `json:"evaluation_ms,omitempty"`
	Version      int                           `json:"version"`
	Items        map[string]*pipelineItemState `json:"items"`
	Receipts     []pipelineReceipt             `json:"receipts"`
}

func (s *pipelineCheckpoint) advance(r TypedRecord) (*pipelineItemState, error) {
	if s == nil || r.Reason == "future" {
		return nil, nil
	}
	item := s.Items[r.ItemKey]
	if item == nil {
		item = &pipelineItemState{Steps: map[int]pipelineStepState{}}
		s.Items[r.ItemKey] = item
	}
	if r.ObservedAtMS <= item.ObservedAtMS {
		return nil, errors.New("pipeline_observation_conflict")
	}
	item.ObservedAtMS = r.ObservedAtMS
	if !definitionEnum(r.State, "ok", "problem") {
		clear(item.Steps)
	}
	return item, nil
}

func (s *pipelineCheckpoint) lookup(digest string) json.RawMessage {
	for _, receipt := range s.Receipts {
		if receipt.InputDigest == digest {
			return receipt.Result
		}
	}
	return nil
}

func (s *pipelineCheckpoint) appendReceipt(digest string, result json.RawMessage) ([]byte, error) {
	if len(s.Receipts) >= pipelineReplayWindow {
		s.Receipts = s.Receipts[1:]
	}
	s.Receipts = append(s.Receipts, pipelineReceipt{InputDigest: digest, Result: result})
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > MaxPipelineCheckpointBytes {
		return nil, errors.New("pipeline_checkpoint_limit")
	}
	return raw, nil
}

func decodePipelineCheckpoint(raw []byte, assigned map[string]bool, steps int) (pipelineCheckpoint, error) {
	if raw == nil {
		return pipelineCheckpoint{Version: 1, Items: map[string]*pipelineItemState{}}, nil
	}
	invalid := errors.New("pipeline_checkpoint_invalid")
	var state pipelineCheckpoint
	if len(raw) > MaxPipelineCheckpointBytes {
		return state, invalid
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if !json.Valid(raw) || dec.Decode(&state) != nil || state.Version != 1 || state.Items == nil ||
		!definitionInteger(state.EvaluationMS, 0, 253402300799999) || len(state.Items) > len(assigned) || len(state.Receipts) > pipelineReplayWindow {
		return state, invalid
	}
	for key, item := range state.Items {
		if !assigned[key] || !validPipelineCheckpointItem(item, steps) {
			return state, invalid
		}
	}
	return state, nil
}

func validPipelineCheckpointItem(item *pipelineItemState, steps int) bool {
	if item == nil || item.Steps == nil || !definitionInteger(item.ObservedAtMS, 1, 253402300799999) {
		return false
	}
	for index, step := range item.Steps {
		if index < 0 || index >= steps || !definitionInteger(step.At, 1, item.ObservedAtMS) {
			return false
		}
	}
	return true
}
