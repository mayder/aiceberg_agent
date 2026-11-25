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
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

var errRelayProbeBadRequest = errors.New("relay_probe_bad_request")
var errRelayAssignmentConflict = errors.New("relay_assignment_http_conflict")

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
	body := map[string]any{"command_id": commandID}
	if httpx.VerifiedHTTPS(c.cfg.HubURL, c.cfg.TLSInsecureSkip) {
		body["credential_mode"] = entities.AgentlessCredentialReferenceV1
	}
	err := c.exchange(ctx, relayprobe.JobPath, body, &out)
	// Older APIs reject the extra key before claiming the job.
	if errors.Is(err, errRelayProbeBadRequest) && body["credential_mode"] != nil {
		delete(body, "credential_mode")
		out = entities.RelaySNMPJob{}
		err = c.exchange(ctx, relayprobe.JobPath, body, &out)
	}
	if err == nil && out.Status == "ready" && ((c.cfg.AgentID > 0 && out.ExecutorID != c.cfg.AgentID) || (c.cfg.AgentClientID > 0 && (out.Job == nil || out.Job.ClienteID != c.cfg.AgentClientID))) {
		err = errors.New("relay_probe_identity_mismatch")
	}
	if out.Job != nil && out.Job.CredentialDelivery != "" && out.Job.SNMP != nil {
		metadata := out.Job.SNMP.WithoutCredential()
		out.Job.SNMP = &metadata
	}
	return out, err
}

func (c *RelaySNMPProbeClient) ResolveSNMPCredentials(ctx context.Context, job entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
	if !httpx.VerifiedHTTPS(c.cfg.HubURL, c.cfg.TLSInsecureSkip) || job.CommandID == "" ||
		job.SNMP == nil || job.Endpoint == nil || job.ProbePolicy == nil {
		return nil, errSNMPCredentialUnavailable
	}
	lookup := snmpCredentialLookup(job)
	var raw json.RawMessage
	if err := c.exchange(ctx, relayprobe.JobPath, lookup, &raw); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errSNMPCredentialUnavailable
	}
	return decodeSNMPCredential(bytes.NewReader(raw), *job.SNMP, lookup.CredentialRef, lookup.CommandID)
}

func (c *RelaySNMPProbeClient) ReportProbe(ctx context.Context, observation entities.AgentlessObservation) error {
	var out struct {
		Status   string `json:"status"`
		Accepted bool   `json:"accepted"`
	}
	body := map[string]any{"command_id": observation.CommandID, "observation": observation}
	if err := c.exchange(ctx, relayprobe.ResultPath, body, &out); err != nil {
		return err
	}
	if out.Status != "ok" || !out.Accepted {
		return errors.New("relay_probe_not_acknowledged")
	}
	return nil
}

func (c *RelaySNMPProbeClient) exchange(ctx context.Context, path string, body any, out any) error {
	target := strings.TrimRight(c.cfg.HubURL, "/") + path
	if relayprobe.IsAssignmentPath(path) && c.cfg.RelaySNMPHubReturnEnabled() && c.cfg.RelaySNMPAssignmentEnabled() {
		target = c.cfg.APIEndpoint(path)
	} else if c.cfg.Mode() != "relay" || strings.TrimSpace(c.cfg.HubURL) == "" {
		return errors.New("relay_probe_hub_required")
	}
	limit := relayprobe.BodyLimit(path)
	if relayprobe.IsAssignmentPath(path) && !httpx.VerifiedHTTPS(target, c.cfg.TLSInsecureSkip) {
		return errors.New("relay_assignment_verified_hub_required")
	}
	raw, err := json.Marshal(body)
	if err != nil || int64(len(raw)) > limit {
		return errors.New("relay_probe_payload_invalid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
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
	client := *c.cl
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("relay_probe_transport_failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest {
		return errRelayProbeBadRequest
	}
	fenced := path == relayprobe.AssignmentResultPath && resp.StatusCode == http.StatusConflict
	if resp.StatusCode != http.StatusOK && !fenced {
		return errors.New("relay_probe_http_failed")
	}
	raw, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return errors.New("relay_probe_response_invalid")
	}
	if err = json.Unmarshal(raw, out); err != nil {
		return errors.New("relay_probe_response_invalid")
	}
	if fenced {
		return errRelayAssignmentConflict
	}
	return nil
}
