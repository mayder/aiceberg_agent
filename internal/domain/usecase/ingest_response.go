package usecase

import (
	"bytes"
	"encoding/json"
)

type IngestConfig struct {
	Raw     json.RawMessage
	Payload *ConfigPayload
}

type IngestBackpressure struct {
	RetryAfterSec      int
	SuggestedBatchSize int
	DegradedEndpoint   string
	ErrorsByReason     map[string]int
}

type IngestConfigHandler func(authHeader string, cfg IngestConfig)

func parseIngestConfig(body []byte) (*IngestConfig, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	var wrapper struct {
		Config json.RawMessage `json:"config,omitempty"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	raw := bytes.TrimSpace(wrapper.Config)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	cfg := &IngestConfig{Raw: raw}
	var payload ConfigPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return cfg, err
	}
	cfg.Payload = &payload
	return cfg, nil
}

func parseIngestBackpressure(body []byte) (*IngestBackpressure, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	var wrapper struct {
		RetryAfter         int            `json:"retry_after,omitempty"`
		SuggestedBatchSize int            `json:"suggested_batch_size,omitempty"`
		DegradedEndpoint   string         `json:"degraded_endpoint,omitempty"`
		ErrorsByReason     map[string]int `json:"errors_by_reason,omitempty"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	if wrapper.RetryAfter <= 0 && wrapper.SuggestedBatchSize <= 0 && wrapper.DegradedEndpoint == "" && len(wrapper.ErrorsByReason) == 0 {
		return nil, nil
	}
	return &IngestBackpressure{
		RetryAfterSec:      wrapper.RetryAfter,
		SuggestedBatchSize: wrapper.SuggestedBatchSize,
		DegradedEndpoint:   wrapper.DegradedEndpoint,
		ErrorsByReason:     wrapper.ErrorsByReason,
	}, nil
}
