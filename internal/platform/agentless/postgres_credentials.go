package agentless

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

var errPostgresCredential = errors.New("postgresql_credential_unavailable")

type postgresProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Database   string `json:"database"`
	User       string `json:"user"`
	Password   string `json:"password"`
	CAPEM      string `json:"ca_pem,omitempty"`
}

func loadPostgresProfile(path string, job entities.AgentlessJob, selection monitoring.PostgresSelection) (postgresProfile, error) {
	host, port, ok := probeTCPEndpoint(job.Endpoint, 5432)
	if !ok || job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 ||
		job.ProbePolicy == nil || job.ProbePolicy.CredentialRef != selection.CredentialRef {
		return postgresProfile{}, errPostgresCredential
	}
	raw, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return postgresProfile{}, errPostgresCredential
	}
	var file struct {
		Profiles []postgresProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return postgresProfile{}, errPostgresCredential
	}
	var match *postgresProfile
	for _, profile := range file.Profiles {
		if profile.Reference != selection.CredentialRef {
			continue
		}
		if match != nil {
			return postgresProfile{}, errPostgresCredential
		}
		match = &profile
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID || match.CheckID != job.CheckID ||
		match.EndpointID != job.Endpoint.ID || match.Host != host || match.Port != port || match.Database != selection.Database ||
		!monitoring.ValidPostgresIdentity(match.User) || match.Password == "" || len(match.Password) > 1024 ||
		strings.ContainsRune(match.Password, 0) {
		return postgresProfile{}, errPostgresCredential
	}
	return *match, nil
}
