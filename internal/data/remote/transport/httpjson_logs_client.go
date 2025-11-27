package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// HTTP client específico para logs brutos.
type logsClient struct {
	cl  *http.Client
	cfg config.Config
}

func NewHTTPLogsClient(cfg config.Config) ports.Transport {
	return &logsClient{
		cl:  &http.Client{Timeout: 10 * time.Second},
		cfg: cfg,
	}
}

func (h *logsClient) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	b, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, h.cfg.APIEndpoint("/v1/logs/raw"), bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	} else if h.cfg.Agent.Token != "" {
		req.Header.Set("Authorization", "Token "+h.cfg.Agent.Token)
	} else if h.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)
	}
	resp, err := h.cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &httpStatusErr{code: resp.StatusCode}
	}
	return nil
}
