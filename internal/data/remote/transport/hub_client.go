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

// HubClient envia lotes para um hub (relay).
type hubClient struct {
	cl  *http.Client
	cfg config.Config
}

func NewHubClient(cfg config.Config) ports.Transport {
	return &hubClient{
		cl:  &http.Client{Timeout: 10 * time.Second},
		cfg: cfg,
	}
}

func (h *hubClient) SendWithAuth(batch []entities.Envelope, authHeader string) error {
	b, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	url := h.cfg.HubURL + "/v1/ingest"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
