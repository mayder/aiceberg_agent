package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// Relay diagnostics have no direct-cloud fallback.
type RelaySNMPProbeClient struct {
	cfg      config.Config
	cl       *http.Client
	identity func() string
}

func NewRelaySNMPProbeClient(cfg config.Config) *RelaySNMPProbeClient {
	return &RelaySNMPProbeClient{cfg: cfg, cl: httpx.NewClient(cfg, 8*time.Second), identity: func() string { return cfg.AgentIdentityHeader("") }}
}

func NewRelaySNMPProbeClientWithIdentity(cfg config.Config, provider func() string) *RelaySNMPProbeClient {
	client := NewRelaySNMPProbeClient(cfg)
	if provider != nil {
		client.identity = provider
	}
	return client
}

func (c *RelaySNMPProbeClient) FetchProbe(ctx context.Context, commandID string) (entities.RelaySNMPJob, error) {
	var out entities.RelaySNMPJob
	err := c.exchange(ctx, "/v1/relay-snmp-probe/job", map[string]any{"command_id": commandID}, &out)
	if err == nil && out.Status == "ready" && ((c.cfg.AgentID > 0 && out.ExecutorID != c.cfg.AgentID) || (c.cfg.AgentClientID > 0 && (out.Job == nil || out.Job.ClienteID != c.cfg.AgentClientID))) {
		err = errors.New("relay_probe_identity_mismatch")
	}
	return out, err
}

func (c *RelaySNMPProbeClient) ReportProbe(ctx context.Context, observation entities.AgentlessObservation) error {
	var out struct {
		Status   string `json:"status"`
		Accepted bool   `json:"accepted"`
	}
	body := map[string]any{"command_id": observation.CommandID, "observation": observation}
	if err := c.exchange(ctx, "/v1/relay-snmp-probe/result", body, &out); err != nil {
		return err
	}
	if out.Status != "ok" || !out.Accepted {
		return errors.New("relay_probe_not_acknowledged")
	}
	return nil
}

func (c *RelaySNMPProbeClient) exchange(ctx context.Context, path string, body any, out any) error {
	if c.cfg.Mode() != "relay" || strings.TrimSpace(c.cfg.HubURL) == "" {
		return errors.New("relay_probe_hub_required")
	}
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 16384 {
		return errors.New("relay_probe_payload_invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.HubURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return errors.New("relay_probe_request_invalid")
	}
	httpx.SetAuth(req, c.cfg)
	req.Header.Set("Content-Type", "application/json")
	identity := c.identity()
	if identity == "" {
		return errors.New("relay_probe_identity_required")
	}
	req.Header.Set("X-Agent-Identity", identity)
	resp, err := c.cl.Do(req)
	if err != nil {
		return errors.New("relay_probe_transport_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("relay_probe_http_failed")
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return errors.New("relay_probe_response_invalid")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return errors.New("relay_probe_response_invalid")
	}
	return nil
}
