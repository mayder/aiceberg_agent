package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

var errSNMPCredentialUnavailable = errors.New("snmp_credential_unavailable")

type snmpCredentialRequest struct {
	Operation     string `json:"operation"`
	CheckID       int    `json:"check_id"`
	AssetID       int    `json:"ativo_id"`
	EndpointID    int    `json:"endpoint_id"`
	BindingID     int    `json:"binding_id"`
	CredentialRef string `json:"credential_ref"`
	Version       string `json:"version"`
	Address       string `json:"endereco"`
	Port          int    `json:"port"`
	CommandID     string `json:"command_id,omitempty"`
}

func (c *AgentlessHubClient) canResolveSNMPCredentials(endpoint string) bool {
	return httpx.VerifiedHTTPS(endpoint, c.cfg.TLSInsecureSkip)
}

func snmpCredentialLookup(job entities.AgentlessJob) snmpCredentialRequest {
	return snmpCredentialRequest{Operation: "resolve_snmp_profile", CheckID: job.CheckID, AssetID: job.AtivoID,
		EndpointID: job.Endpoint.ID, BindingID: job.SNMP.BindingID, CredentialRef: job.ProbePolicy.CredentialRef,
		Version: job.SNMP.Version, Address: job.Endpoint.Endereco, Port: job.SNMP.Port, CommandID: job.CommandID}
}

// ResolveSNMPCredentials returns a transient profile. It never mutates the cached job.
func (c *AgentlessHubClient) ResolveSNMPCredentials(ctx context.Context, job entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
	endpoint := c.cfg.APIEndpoint("/v1/hub-agentless/jobs")
	if !c.canResolveSNMPCredentials(endpoint) || job.SNMP == nil || job.Endpoint == nil || job.ProbePolicy == nil {
		return nil, errSNMPCredentialUnavailable
	}
	lookup := snmpCredentialLookup(job)
	payload, err := json.Marshal(lookup)
	if err != nil {
		return nil, errSNMPCredentialUnavailable
	}
	// Keep the control-plane request bounded even for legacy collection budgets.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?lock=0&limit=1", bytes.NewReader(payload))
	if err != nil {
		return nil, errSNMPCredentialUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAgentHeaders(req)
	client := *c.cl
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errSNMPCredentialUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errSNMPCredentialUnavailable
	}
	return decodeSNMPCredential(response.Body, *job.SNMP, lookup.CredentialRef, lookup.CommandID)
}

func decodeSNMPCredential(body io.Reader, profile entities.AgentlessSnmpProfile, reference, commandID string) (*entities.AgentlessSnmpProfile, error) {
	var result struct {
		Status        string `json:"status"`
		CredentialRef string `json:"credential_ref"`
		CommandID     string `json:"command_id"`
		Credential    struct {
			Community      string `json:"community"`
			V3User         string `json:"v3_user"`
			V3AuthProtocol string `json:"v3_auth_protocol"`
			V3AuthPassword string `json:"v3_auth_password"`
			V3PrivProtocol string `json:"v3_priv_protocol"`
			V3PrivPassword string `json:"v3_priv_password"`
			ContextName    string `json:"context_name"`
		} `json:"credential"`
	}
	raw, err := io.ReadAll(io.LimitReader(body, 8193))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err != nil || len(raw) > 8192 || decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF ||
		result.Status != "ok" || result.CredentialRef != reference || result.CommandID != commandID {
		return nil, errSNMPCredentialUnavailable
	}
	profile = profile.WithoutCredential()
	credential := result.Credential
	switch profile.Version {
	case "v1", "v2c":
		if credential.Community == "" {
			return nil, errSNMPCredentialUnavailable
		}
		profile.Community = credential.Community
	case "v3":
		if credential.V3User == "" {
			return nil, errSNMPCredentialUnavailable
		}
		profile.V3User, profile.ContextName = credential.V3User, credential.ContextName
		profile.V3AuthProtocol, profile.V3AuthPassword = credential.V3AuthProtocol, credential.V3AuthPassword
		profile.V3PrivProtocol, profile.V3PrivPassword = credential.V3PrivProtocol, credential.V3PrivPassword
	default:
		return nil, errSNMPCredentialUnavailable
	}
	return &profile, nil
}
