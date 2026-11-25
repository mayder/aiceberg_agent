package agentless

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type mysqlProbeProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Password   string `json:"password"`
	CAPEM      string `json:"ca_pem,omitempty"`
}

var mysqlProbeUsername = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,31}$`)

func loadMySQLProfile(path string, job entities.AgentlessJob, selection monitoring.MySQLSelection) (mysqlProbeProfile, bool) {
	host, port, valid := probeTCPEndpoint(job.Endpoint, 3306)
	if !valid || job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 ||
		job.ProbePolicy == nil || job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return mysqlProbeProfile{}, false
	}
	raw, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return mysqlProbeProfile{}, false
	}
	defer clear(raw)
	var file struct {
		Profiles []mysqlProbeProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return mysqlProbeProfile{}, false
	}
	var match *mysqlProbeProfile
	for _, profile := range file.Profiles {
		if profile.Reference != selection.CredentialRef {
			continue
		}
		if match != nil {
			return mysqlProbeProfile{}, false
		}
		match = &profile
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.Host != host || match.Port != port || !mysqlProbeUsername.MatchString(match.User) ||
		len(match.Password) < 1 || len(match.Password) > 1024 || strings.ContainsRune(match.Password, 0) {
		return mysqlProbeProfile{}, false
	}
	return *match, true
}
