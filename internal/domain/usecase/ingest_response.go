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

type IngestAckResult struct {
	Received       int
	Skipped        int
	Status         string
	ErrorsByReason map[string]int
	HasAckFields   bool
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

func parseIngestAckResult(body []byte) (*IngestAckResult, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	var wrapper struct {
		Received       *int           `json:"received,omitempty"`
		Skipped        *int           `json:"skipped,omitempty"`
		Status         string         `json:"status,omitempty"`
		ErrorsByReason map[string]int `json:"errors_by_reason,omitempty"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	hasAckFields := wrapper.Received != nil || wrapper.Skipped != nil || len(wrapper.ErrorsByReason) > 0
	if !hasAckFields {
		return nil, nil
	}
	result := &IngestAckResult{
		Status:         wrapper.Status,
		ErrorsByReason: wrapper.ErrorsByReason,
		HasAckFields:   hasAckFields,
	}
	if wrapper.Received != nil {
		result.Received = *wrapper.Received
	}
	if wrapper.Skipped != nil {
		result.Skipped = *wrapper.Skipped
	}
	return result, nil
}

func ingestAckIsSafe(result *IngestAckResult, batchSize int) bool {
	if result == nil || !result.HasAckFields || batchSize <= 0 {
		return true
	}
	if result.Received >= batchSize {
		return true
	}
	if result.Received < 0 || result.Skipped < 0 {
		return false
	}
	if result.Received+result.Skipped < batchSize {
		return false
	}
	for reason, count := range result.ErrorsByReason {
		if count <= 0 {
			continue
		}
		if !isSafeIngestSkipReason(reason) {
			return false
		}
	}
	return result.Received+safeSkippedCount(result) >= batchSize
}

func safeSkippedCount(result *IngestAckResult) int {
	if result == nil || result.Skipped <= 0 {
		return 0
	}
	if len(result.ErrorsByReason) == 0 {
		return 0
	}
	total := 0
	for reason, count := range result.ErrorsByReason {
		if count > 0 && isSafeIngestSkipReason(reason) {
			total += count
		}
	}
	if total > result.Skipped {
		return result.Skipped
	}
	return total
}

func isSafeIngestSkipReason(reason string) bool {
	switch reason {
	case "duplicate_envelope_id", "invalid_envelope":
		return true
	default:
		return false
	}
}
