package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type vcenterProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	URL        string `json:"url"`
	VMID       string `json:"vm_id"`
	VMUUID     string `json:"vm_uuid"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	CAPEM      string `json:"ca_pem,omitempty"`
}

func vcenterEndpoint(endpoint *entities.AgentlessEndpoint) (*url.URL, bool) {
	if endpoint == nil || endpoint.ID < 1 {
		return nil, false
	}
	address, err := probeHTTPURL(endpoint)
	if err != nil {
		return nil, false
	}
	target, err := url.Parse(address)
	return target, err == nil && target.Scheme == "https" && target.EscapedPath() == "/sdk"
}

func loadVCenterProfile(path string, job entities.AgentlessJob, selection monitoring.VCenterSelection, target *url.URL) (vcenterProfile, bool) {
	if job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 || job.ProbePolicy == nil ||
		job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return vcenterProfile{}, false
	}
	raw, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return vcenterProfile{}, false
	}
	defer clear(raw)
	var file struct {
		Profiles []vcenterProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return vcenterProfile{}, false
	}
	var match *vcenterProfile
	for _, profile := range file.Profiles {
		if profile.Reference != selection.CredentialRef {
			continue
		}
		if match != nil {
			return vcenterProfile{}, false
		}
		match = &profile
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.URL != target.String() ||
		match.VMID != selection.VMID || match.VMUUID != selection.VMUUID ||
		!validVCenterSecret(match.Username, 256) || !validVCenterSecret(match.Password, 1024) {
		return vcenterProfile{}, false
	}
	return *match, true
}

func validVCenterSecret(value string, maxBytes int) bool {
	return len(value) > 0 && len(value) <= maxBytes && strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) == -1
}
