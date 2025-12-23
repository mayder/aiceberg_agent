package usecase

import (
	"bytes"
	"encoding/json"
)

type IngestConfig struct {
	Raw     json.RawMessage
	Payload *ConfigPayload
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
