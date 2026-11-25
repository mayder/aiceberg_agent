package agentless

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestJolokiaReadsExactAttributesInOneRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var reads []jolokiaRead
		if request.Method != http.MethodPost || request.URL.Path != "/jolokia" ||
			json.NewDecoder(request.Body).Decode(&reads) != nil || len(reads) != 2 {
			t.Error("expected one bulk read request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if reads[0] != (jolokiaRead{Type: "read", MBean: "java.lang:type=Memory", Attribute: "HeapMemoryUsage", Path: "used"}) ||
			reads[1] != (jolokiaRead{Type: "read", MBean: "java.lang:name=G1 Young Generation,type=GarbageCollector", Attribute: "CollectionCount"}) {
			t.Error("configured exact selectors were not sent")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, `[{"status":200,"timestamp":%d,"value":0},{"status":403,"error":"private-fixture","value":300}]`, time.Now().Unix())
	}))
	defer server.Close()
	obs := RunJob(context.Background(), jolokiaTestJob(server.URL+"/jolokia"))
	if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "jolokia" ||
		obs.MonitoringSamples == nil || calls.Load() != 1 {
		t.Fatalf("failed to produce the bulk observation: %s", obs.Code)
	}
	samples := obs.MonitoringSamples.Samples
	if len(samples) != 2 || samples[0].Value == nil || *samples[0].Value != 0 ||
		samples[1].Value != nil || samples[1].Gap != "missing" {
		t.Fatalf("wrong success or error projection: %+v", samples)
	}
	wire, _ := json.Marshal(obs)
	if strings.Contains(string(wire), "private-fixture") || strings.Contains(string(wire), "GarbageCollector") {
		t.Fatal("raw exporter response or selector leaked")
	}
}

func TestJolokiaRejectsTamperedResponsesAndOversize(t *testing.T) {
	selectors, err := monitoring.JolokiaSelectors(jolokiaTestJob("").Config)
	if err != nil {
		t.Fatal(err)
	}
	requests := make([]jolokiaRead, 0, len(selectors))
	for _, selector := range selectors {
		requests = append(requests, jolokiaRead{Type: "read", MBean: selector.MBean, Attribute: selector.Attribute, Path: selector.Path})
	}
	for _, body := range []string{
		`[{"status":200,"value":7}]`,
		`[{"status":403},{"status":404}]`,
		`[{"status":200,"request":{"type":"write","mbean":"java.lang:type=Memory","attribute":"HeapMemoryUsage","path":"used"}},{"status":200}]`,
		strings.Repeat(" ", maxJolokiaBytes+1),
	} {
		if _, err := readJolokia(strings.NewReader(body), requests, selectors, 180, time.Now().Unix()); err == nil {
			t.Fatal("invalid exporter response accepted")
		}
	}
}

func TestJolokiaReadsRealJVM(t *testing.T) {
	raw := os.Getenv("AICEBERG_JOLOKIA_TEST_URL")
	if raw == "" {
		t.Skip("starts only with a dedicated local Jolokia fixture")
	}
	endpoint, err := url.Parse(raw)
	if err != nil || net.ParseIP(endpoint.Hostname()) == nil || !net.ParseIP(endpoint.Hostname()).IsLoopback() {
		t.Fatal("fixture must be a local JVM")
	}
	job := jolokiaTestJob(raw)
	job.Config = entities.AgentlessConfig{"metrics": []any{
		map[string]any{"id": "heap", "mbean": "java.lang:type=Memory", "attribute": "HeapMemoryUsage", "path": "used"},
		map[string]any{"id": "threads", "mbean": "java.lang:type=Threading", "attribute": "ThreadCount", "path": ""},
	}}
	if reference := os.Getenv("AICEBERG_JOLOKIA_TEST_HTTP_REFERENCE"); reference != "" {
		job.Config["credential_ref"] = reference
		job.ProbePolicy.CredentialRef = reference
	}
	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" || obs.MonitoringSamples == nil || len(obs.MonitoringSamples.Samples) != 2 {
		t.Fatalf("live JVM probe failed: %s", obs.Code)
	}
	for _, sample := range obs.MonitoringSamples.Samples {
		if sample.Value == nil || *sample.Value <= 0 {
			t.Fatalf("missing or implausible JVM measurement: %s", sample.Metric)
		}
	}
	if job.ProbePolicy.CredentialRef != "" {
		verifyRealJolokiaAuthentication(t, job, endpoint, obs)
	}
}

func verifyRealJolokiaAuthentication(t *testing.T, job entities.AgentlessJob, endpoint *url.URL, accepted entities.AgentlessObservation) {
	t.Helper()
	originalPath := os.Getenv("AICEBERG_HTTP_PROBE_PROFILES_FILE")
	profile, ok := loadHTTPProbeProfile(originalPath, job, &http.Request{URL: endpoint}, job.ProbePolicy.CredentialRef)
	if !ok {
		t.Fatal("authenticated JVM fixture requires a scoped HTTPS profile")
	}
	assertHTTPProbeRedacted(t, accepted, profile)
	foreign := job
	foreign.ClienteID++
	if observation := RunJob(context.Background(), foreign); observation.Code != httpCredentialUnavailable {
		t.Fatal("different client obtained access to the JVM")
	}
	wrong := profile
	wrong.Password = "fixture-invalid-password"
	writeFixtureHTTPProfiles(t, wrong)
	rejected := RunJob(context.Background(), job)
	if rejected.Code != httpAccessDenied || rejected.ProbeReport == nil || rejected.ProbeReport.Reason != "configuration" {
		t.Fatalf("the real JVM did not reject the invalid credential: %s", rejected.Code)
	}
	assertHTTPProbeRedacted(t, rejected, wrong)
	untrusted := profile
	untrusted.CAPEM = ""
	writeFixtureHTTPProfiles(t, untrusted)
	if observation := RunJob(context.Background(), job); observation.Code != "http_fail" {
		t.Fatal("the real JVM's untrusted certificate was accepted")
	}
	t.Setenv("AICEBERG_HTTP_PROBE_PROFILES_FILE", originalPath)
	recovered := RunJob(context.Background(), job)
	if recovered.Status != "ok" || recovered.MonitoringSamples == nil || len(recovered.MonitoringSamples.Samples) != 2 {
		t.Fatalf("the JVM did not recover with the scoped credential and CA: %s", recovered.Code)
	}
	assertHTTPProbeRedacted(t, recovered, profile)
}

func jolokiaTestJob(target string) entities.AgentlessJob {
	return entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "jolokia", BaseIntervalSec: 60,
		Endpoint:    &entities.AgentlessEndpoint{ID: 17, Tipo: "url", Endereco: target},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 3000, FreshnessSec: 180},
		Config: entities.AgentlessConfig{"metrics": []any{
			map[string]any{"id": "heap", "mbean": "java.lang:type=Memory", "attribute": "HeapMemoryUsage", "path": "used"},
			map[string]any{"id": "gc", "mbean": "java.lang:type=GarbageCollector,name=G1 Young Generation", "attribute": "CollectionCount", "path": ""},
		}},
	}
}
