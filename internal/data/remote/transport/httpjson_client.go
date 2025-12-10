package transport

import (
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type httpClient struct {
	cl  *http.Client
	cfg config.Config
}

func NewHTTPJSONClient(cfg config.Config) ports.Transport {
	return &httpClient{
		cl:  httpx.NewClient(cfg, 10*time.Second),
		cfg: cfg,
	}
}

func (h *httpClient) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	req, err := buildRequest(h.cfg.APIEndpoint("/v1/ingest"), batch, h.cfg)
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

type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return http.StatusText(e.code) }
