package agentless

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/safefile"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const httpCredentialUnavailable = "http_credential_unavailable"
const httpAccessDenied = "http_access_denied"

var bearerCredential = regexp.MustCompile(`^[A-Za-z0-9._~+/\-]+=*$`)

type httpProbeProfile struct {
	Reference  string `json:"reference"`
	ClientID   int    `json:"client_id"`
	AssetID    int    `json:"asset_id"`
	CheckID    int    `json:"check_id"`
	EndpointID int    `json:"endpoint_id"`
	Protocol   string `json:"protocol"`
	URL        string `json:"url"`
	Method     string `json:"method"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	Token      string `json:"token,omitempty"`
	CAPEM      string `json:"ca_pem,omitempty"`
}

func doNumericHTTPRequest(req *http.Request, job entities.AgentlessJob) (*http.Response, Result) {
	reference, valid := monitoring.HTTPProbeCredential(job.Config)
	if !valid || job.ProbePolicy == nil || job.ProbePolicy.CredentialRef != reference {
		return nil, Result{Status: "fail", Code: httpCredentialUnavailable}
	}
	client := probeHTTPClient
	if reference != "" {
		profile, ok := loadHTTPProbeProfile(config.AgentlessHTTPProbeProfilesFile(), job, req, reference)
		if !ok {
			return nil, Result{Status: "fail", Code: httpCredentialUnavailable}
		}
		ca := &tls.Config{MinVersion: tls.VersionTLS12}
		if profile.CAPEM != "" {
			ca.RootCAs = x509.NewCertPool()
			if !ca.RootCAs.AppendCertsFromPEM([]byte(profile.CAPEM)) {
				return nil, Result{Status: "fail", Code: httpCredentialUnavailable}
			}
		}
		transport := probeHTTPTransport.Clone()
		transport.TLSClientConfig = ca
		// Keep TLS trust and connections within the current credential scope.
		transport.DisableKeepAlives = true
		client = &http.Client{Transport: transport, CheckRedirect: probeHTTPClient.CheckRedirect}
		profile.authorize(req)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, Result{Status: "fail", Code: "http_fail", failure: err}
	}
	if response.StatusCode == http.StatusOK {
		return response, Result{}
	}
	_ = response.Body.Close()
	if reference != "" && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
		return nil, Result{Status: "fail", Code: httpAccessDenied}
	}
	return nil, Result{Status: "fail", Code: "http_status"}
}

func loadHTTPProbeProfile(path string, job entities.AgentlessJob, request *http.Request, reference string) (httpProbeProfile, bool) {
	if request.URL.Scheme != "https" || job.ClienteID < 1 || job.AtivoID < 1 || job.CheckID < 1 ||
		job.Endpoint == nil || job.Endpoint.ID < 1 {
		return httpProbeProfile{}, false
	}
	raw, err := safefile.ReadPrivate(path, 512<<10)
	if err != nil {
		return httpProbeProfile{}, false
	}
	defer clear(raw)
	var file struct {
		Profiles []httpProbeProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || len(file.Profiles) > 256 {
		return httpProbeProfile{}, false
	}
	var match *httpProbeProfile
	for _, profile := range file.Profiles {
		if profile.Reference == reference {
			if match != nil {
				return httpProbeProfile{}, false
			}
			match = &profile
		}
	}
	if match == nil || match.ClientID != job.ClienteID || match.AssetID != job.AtivoID ||
		match.CheckID != job.CheckID || match.EndpointID != job.Endpoint.ID || match.Protocol != job.Tipo ||
		match.URL != request.URL.String() || !match.valid() {
		return httpProbeProfile{}, false
	}
	return *match, true
}

func (p httpProbeProfile) valid() bool {
	switch p.Method {
	case "basic":
		return p.Token == "" && validBasicCredential(p.Username, 128) && !strings.Contains(p.Username, ":") &&
			validBasicCredential(p.Password, 1024)
	case "bearer":
		return p.Username == "" && p.Password == "" && len(p.Token) <= 4096 && bearerCredential.MatchString(p.Token)
	default:
		return false
	}
}

func validBasicCredential(value string, limit int) bool {
	return value != "" && len(value) <= limit &&
		strings.IndexFunc(value, func(r rune) bool { return r < 32 || r == 127 }) < 0
}

func (p httpProbeProfile) authorize(req *http.Request) {
	if p.Method == "basic" {
		req.SetBasicAuth(p.Username, p.Password)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
}
