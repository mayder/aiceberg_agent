package transport

import (
	"io"
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
	end string
}

func NewHTTPJSONClient(cfg config.Config) ports.Transport {
	return &httpClient{
		cl:  httpx.NewClient(cfg, 10*time.Second),
		cfg: cfg,
		end: "/v1/ingest",
	}
}

func NewHTTPJSONClientWithEndpoint(cfg config.Config, endpoint string) ports.Transport {
	if endpoint == "" {
		endpoint = "/v1/ingest"
	}
	return &httpClient{
		cl:  httpx.NewClient(cfg, 10*time.Second),
		cfg: cfg,
		end: endpoint,
	}
}

func (h *httpClient) SendWithAuth(batch []entities.Envelope, authHeader string, endpoint string) ([]byte, error) {
	target := h.end
	if endpoint != "" {
		target = endpoint
	}
	req, err := buildRequest(h.cfg.APIEndpoint(target), batch, h.cfg)
	if err != nil {
		return nil, err
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

type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return http.StatusText(e.code) }
