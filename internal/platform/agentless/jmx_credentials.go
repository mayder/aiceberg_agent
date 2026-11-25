package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"net/netip"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type jmxProfile struct {
	Reference    string `json:"reference"`
	ClientID     int    `json:"client_id"`
	AssetID      int    `json:"asset_id"`
	CheckID      int    `json:"check_id"`
	EndpointID   int    `json:"endpoint_id"`
	Host         string `json:"host"`
	IP           string `json:"ip"`
	RegistryPort int    `json:"registry_port"`
	ServerPort   int    `json:"server_port"`
	User         string `json:"user"`
	Password     string `json:"password"`
	CA           string `json:"ca_pem"`
}

var jmxUsername = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,63}$`)

func loadJMXProfile(path string, job entities.AgentlessJob, selection monitoring.JMXSelection) (jmxProfile, bool) {
	host, port, valid := probeTCPEndpoint(job.Endpoint, 0)
	if !valid || job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 ||
		job.ProbePolicy == nil || job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return jmxProfile{}, false
	}
	body, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return jmxProfile{}, false
	}
	defer clear(body)
	var file struct {
		Profiles []jmxProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return jmxProfile{}, false
	}
	var match *jmxProfile
	for _, profile := range file.Profiles {
		if profile.Reference != selection.CredentialRef {
			continue
		}
		if match != nil {
			return jmxProfile{}, false
		}
		match = &profile
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.Host != host || match.RegistryPort != port || !validJMXProfile(*match) {
		return jmxProfile{}, false
	}
	return *match, true
}

func validJMXProfile(profile jmxProfile) bool {
	ip, err := netip.ParseAddr(profile.IP)
	if err != nil || ip.Zone() != "" || ip.Unmap().String() != profile.IP || !probeHTTPAddressAllowed(profile.Host, ip) ||
		profile.ServerPort < 1 || profile.ServerPort > 65535 ||
		!jmxUsername.MatchString(profile.User) || len(profile.Password) < 1 || len(profile.Password) > 1024 ||
		!utf8.ValidString(profile.Password) || strings.ContainsRune(profile.Password, 0) || len(profile.CA) < 1 || len(profile.CA) > 32768 {
		return false
	}
	if literal, err := netip.ParseAddr(profile.Host); err == nil {
		return literal.Unmap() == ip
	}
	return !ip.IsLoopback()
}
