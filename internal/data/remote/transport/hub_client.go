package transport

import (
	"io"
	"net/http"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

// HubClient envia lotes para um hub (relay).
type hubClient struct {
	cl  *http.Client
	cfg config.Config
	end string
}

func NewHubClient(cfg config.Config) ports.Transport {
	return &hubClient{
		cl:  httpx.NewClient(cfg, ingestTimeout(cfg)),
		cfg: cfg,
		end: "/v1/ingest",
	}
}

func (h *hubClient) SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error) {
	// Sempre envia ao endpoint fixo do hub; o destino real vai dentro do envelope.
	_ = endpoint
	url := h.cfg.HubURL + h.end
	req, err := buildRequest(url, batch, h.cfg)
	if err != nil {
		return nil, err
	}
	setEnvelopeIdentityHeader(req, batch)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	} else if h.cfg.HubToken != "" {
		req.Header.Set("Authorization", "Token "+h.cfg.HubToken)
	}
	resp, err := h.cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, &httpStatusErr{code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return body, nil
}
