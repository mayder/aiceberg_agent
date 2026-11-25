package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestNumericHTTPCredentialsTLSAndExactScope(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("private local HTTP credentials require Linux or macOS")
	}
	for _, kind := range []string{"openmetrics", "jolokia"} {
		t.Run(kind, func(t *testing.T) {
			var count atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if !validFixtureHTTPAuth(kind, r) {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if kind == "openmetrics" {
					w.Header().Set("Content-Type", "text/plain;version=0.0.4")
					_, _ = fmt.Fprint(w, "requests_total{route=\"a,b c\"} 17\n")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `[{"status":200,"timestamp":%d,"value":17},{"status":403}]`, time.Now().Unix())
			}))
			defer server.Close()
			job, profile := fixtureAuthenticatedHTTPProbe(t, kind, server)
			writeFixtureHTTPProfiles(t, profile)
			obs := RunJob(context.Background(), job)
			if obs.Status != "ok" || obs.MonitoringSamples == nil || obs.MonitoringSamples.Samples[0].Value == nil ||
				*obs.MonitoringSamples.Samples[0].Value != 17 || count.Load() != 1 {
				t.Fatalf("authenticated measurement failed: %s", obs.Code)
			}
			assertHTTPProbeRedacted(t, obs, profile)
			for _, mutate := range []func(*entities.AgentlessJob){
				func(job *entities.AgentlessJob) { job.ClienteID++ },
				func(job *entities.AgentlessJob) { job.AtivoID++ },
				func(job *entities.AgentlessJob) { job.CheckID++ },
				func(job *entities.AgentlessJob) { job.Endpoint.ID++ },
				func(job *entities.AgentlessJob) { job.Endpoint.Endereco += "/elsewhere" },
				func(job *entities.AgentlessJob) {
					job.Endpoint.Endereco = strings.Replace(job.Endpoint.Endereco, "https:", "http:", 1)
				},
				func(job *entities.AgentlessJob) { job.ProbePolicy.CredentialRef = "" },
				func(job *entities.AgentlessJob) { delete(job.Config, "credential_ref") },
			} {
				attempt, _ := fixtureAuthenticatedHTTPProbe(t, kind, server)
				mutate(&attempt)
				rejected := RunJob(context.Background(), attempt)
				if rejected.Code != httpCredentialUnavailable {
					t.Fatalf("unexpected credential scope outcome: %s", rejected.Code)
				}
				assertHTTPProbeRedacted(t, rejected, profile)
			}
			if count.Load() != 1 {
				t.Fatal("a request left the HUB after a credential scope mismatch")
			}
		})
	}
}

func TestNumericHTTPCredentialsBlockRedirectAndReportDenial(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("private local HTTP credentials require Linux or macOS")
	}
	var redirected atomic.Int32
	var mode atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer destination.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validFixtureHTTPAuth("openmetrics", r) {
			t.Error("credential missing from authorized endpoint")
		}
		if mode.Load() == 1 {
			http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
		} else {
			http.Error(w, "sensitive fixture response", http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	job, profile := fixtureAuthenticatedHTTPProbe(t, "openmetrics", server)
	writeFixtureHTTPProfiles(t, profile)
	obs := RunJob(context.Background(), job)
	if obs.Code != httpAccessDenied || obs.ProbeReport == nil || obs.ProbeReport.Reason != "configuration" {
		t.Fatalf("authentication denial misclassified: %s", obs.Code)
	}
	assertHTTPProbeRedacted(t, obs, profile)
	mode.Store(1)
	if obs := RunJob(context.Background(), job); obs.Code != "http_status" || redirected.Load() != 0 {
		t.Fatal("redirect was followed")
	}
	t.Setenv("AICEBERG_HTTP_PROBE_PROFILES_FILE", "")
	if obs := RunJob(context.Background(), job); obs.Code != httpCredentialUnavailable {
		t.Fatal("missing file allowed an authenticated request")
	}
	profile.CAPEM = ""
	writeFixtureHTTPProfiles(t, profile)
	if obs := RunJob(context.Background(), job); obs.Code != "http_fail" {
		t.Fatal("untrusted certificate accepted")
	}
}

func fixtureAuthenticatedHTTPProbe(t *testing.T, kind string, server *httptest.Server) (entities.AgentlessJob, httpProbeProfile) {
	t.Helper()
	job := openMetricsTestJob(server.URL + "/metrics")
	if kind == "jolokia" {
		job = jolokiaTestJob(server.URL + "/jolokia")
	}
	const reference = "http_probe_profile:51"
	job.ProbePolicy.CredentialRef = reference
	job.Config["credential_ref"] = reference
	profile := httpProbeProfile{Reference: reference, ClientID: job.ClienteID, AssetID: job.AtivoID, CheckID: job.CheckID,
		EndpointID: job.Endpoint.ID, Protocol: job.Tipo, URL: job.Endpoint.Endereco,
		CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))}
	if kind == "openmetrics" {
		profile.Method, profile.Token = "bearer", "fixture-token-123"
	} else {
		profile.Method, profile.Username, profile.Password = "basic", "fixture-user", "fixture-password"
	}
	return job, profile
}

func writeFixtureHTTPProfiles(t *testing.T, profiles ...httpProbeProfile) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"profiles": profiles})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "http-profiles.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_HTTP_PROBE_PROFILES_FILE", path)
}

func validFixtureHTTPAuth(kind string, r *http.Request) bool {
	if kind == "openmetrics" {
		return r.Method == http.MethodGet && r.Header.Get("Authorization") == "Bearer fixture-token-123"
	}
	user, pass, ok := r.BasicAuth()
	return r.Method == http.MethodPost && ok && user == "fixture-user" && pass == "fixture-password"
}

func assertHTTPProbeRedacted(t *testing.T, observation entities.AgentlessObservation, profile httpProbeProfile) {
	t.Helper()
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{profile.Token, profile.Username, profile.Password, profile.CAPEM, "sensitive fixture response", "Authorization"} {
		if value != "" && bytes.Contains(raw, []byte(value)) {
			t.Fatalf("sensitive content in %s observation", profile.Protocol)
		}
	}
}
