package agentless

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestOpenMetricsCollectorReadsRealExporter(t *testing.T) {
	raw := os.Getenv("AICEBERG_OPENMETRICS_TEST_URL")
	if raw == "" {
		t.Skip("starts only with a dedicated local exporter")
	}
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "https" || !net.ParseIP(endpoint.Hostname()).IsLoopback() {
		t.Fatal("exporter fixture must use HTTPS on a literal loopback address")
	}
	job := openMetricsTestJob(raw)
	job.ProbePolicy.BudgetMS = 3000
	job.ProbePolicy.CredentialRef = "http_probe_profile:51"
	job.Config = entities.AgentlessConfig{
		"credential_ref": job.ProbePolicy.CredentialRef,
		"metrics": []any{
			map[string]any{"id": "goroutines", "name": "go_goroutines", "labels": map[string]any{}},
			map[string]any{"id": "heap", "name": "go_memstats_heap_alloc_bytes", "labels": map[string]any{}},
		},
	}
	profilesPath := os.Getenv("AICEBERG_HTTP_PROBE_PROFILES_FILE")
	profile, ok := loadHTTPProbeProfile(profilesPath, job, &http.Request{URL: endpoint}, job.ProbePolicy.CredentialRef)
	if !ok || profile.Method != "basic" || profile.CAPEM == "" {
		t.Fatal("fixture requires a scoped Basic profile and a private CA")
	}
	assertExporterSamples(t, job, profile)
	verifyExporterCredentialRejections(t, job, profile)
	t.Setenv("AICEBERG_HTTP_PROBE_PROFILES_FILE", profilesPath)
	assertExporterSamples(t, job, profile)
}

func verifyExporterCredentialRejections(t *testing.T, job entities.AgentlessJob, profile httpProbeProfile) {
	t.Helper()
	foreign := job
	foreign.ClienteID++
	if observation := RunJob(context.Background(), foreign); observation.Code != httpCredentialUnavailable {
		t.Fatal("different client obtained access to the exporter")
	}
	wrong := profile
	wrong.Password = "fixture-invalid-password"
	writeFixtureHTTPProfiles(t, wrong)
	rejected := RunJob(context.Background(), job)
	if rejected.Status != "fail" || rejected.Code != httpAccessDenied || rejected.ProbeReport == nil ||
		rejected.ProbeReport.Reason != "configuration" || rejected.MonitoringSamples == nil ||
		len(rejected.MonitoringSamples.Samples) != 2 {
		t.Fatalf("invalid credential did not result in a configuration failure: %s", rejected.Code)
	}
	for _, sample := range rejected.MonitoringSamples.Samples {
		if sample.Value != nil || sample.Gap != "observation_failed" {
			t.Fatal("invalid credential returned a usable metric")
		}
	}
	assertHTTPProbeRedacted(t, rejected, wrong)
	untrusted := profile
	untrusted.CAPEM = ""
	writeFixtureHTTPProfiles(t, untrusted)
	if observation := RunJob(context.Background(), job); observation.Code != "http_fail" {
		t.Fatal("the exporter's untrusted certificate was accepted")
	}
}

func assertExporterSamples(t *testing.T, job entities.AgentlessJob, profile httpProbeProfile) {
	t.Helper()
	observation := RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 2 {
		t.Fatalf("exporter collection failed: %s", observation.Code)
	}
	for _, sample := range observation.MonitoringSamples.Samples {
		if sample.Value == nil || *sample.Value <= 0 {
			t.Fatalf("missing or nonpositive Go runtime metric: %s", sample.Metric)
		}
	}
	assertHTTPProbeRedacted(t, observation, profile)
}
