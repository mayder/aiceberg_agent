package transport

import (
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
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
		cl:  httpx.NewClient(cfg, 10*time.Second),
		cfg: cfg,
	}
}

func (h *logsClient) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	req, err := buildRequest(h.cfg.APIEndpoint("/v1/logs/raw"), batch, h.cfg)
	if err != nil {
		return err
	}
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
