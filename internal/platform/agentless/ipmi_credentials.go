package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"net/netip"
	"regexp"
	"strings"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

var validIPMIUser = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,15}$`)

type ipmiProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

func ipmiEndpoint(endpoint *entities.AgentlessEndpoint) (netip.Addr, int, bool) {
	if endpoint == nil || endpoint.ID < 1 || endpoint.Tipo != "ip" {
		return netip.Addr{}, 0, false
	}
	ip, err := netip.ParseAddr(endpoint.Endereco)
	if err != nil || ip.Zone() != "" || !probeHTTPAddressAllowed(endpoint.Endereco, ip.Unmap()) {
		return netip.Addr{}, 0, false
	}
	port := 623
	if endpoint.Porta != nil {
		port = *endpoint.Porta
	}
	return ip.Unmap(), port, port > 0 && port <= 65535
}

func loadIPMIProfile(path string, job entities.AgentlessJob, selection monitoring.IPMISelection) (ipmiProfile, bool) {
	ip, port, ok := ipmiEndpoint(job.Endpoint)
	if !ok || job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 || job.ProbePolicy == nil ||
		job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return ipmiProfile{}, false
	}
	raw, err := safefile.ReadPrivate(path, 128<<10)
	if err != nil {
		return ipmiProfile{}, false
	}
	defer clear(raw)
	var file struct {
		Profiles []ipmiProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return ipmiProfile{}, false
	}
	var match *ipmiProfile
	for _, profile := range file.Profiles {
		if profile.Reference == selection.CredentialRef {
			if match != nil {
				return ipmiProfile{}, false
			}
			match = &profile
		}
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.IP != ip.String() || match.Port != port ||
		!validIPMIUser.MatchString(match.Username) || match.Password == "" || len(match.Password) > 20 ||
		strings.ContainsAny(match.Password, "\x00\r\n") {
		return ipmiProfile{}, false
	}
	return *match, true
}
