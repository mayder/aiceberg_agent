package transport

import (
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// HubClient envia lotes para um hub (relay).
type hubClient struct {
	cl  *http.Client
	cfg config.Config
}

func NewHubClient(cfg config.Config) ports.Transport {
	return &hubClient{
		cl:  httpx.NewClient(cfg, 10*time.Second),
		cfg: cfg,
	}
}

func (h *hubClient) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	url := h.cfg.HubURL + "/v1/ingest"
	req, err := buildRequest(url, batch, h.cfg)
	if err != nil {
		return err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	} else if h.cfg.HubToken != "" {
		req.Header.Set("Authorization", "Token "+h.cfg.HubToken)
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
