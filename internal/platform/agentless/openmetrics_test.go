package agentless

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestOpenMetricsEgressUsesOnlyAuthorizedEndpoints(t *testing.T) {
	for _, address := range []string{"169.254.169.254", "100.100.100.200", "fd00:ec2::254", "127.0.0.1", "::1", "224.0.0.1"} {
		if probeHTTPAddressAllowed("exporter.fixture", netip.MustParseAddr(address)) {
			t.Fatalf("invalid DNS target accepted: %s", address)
		}
	}
	if !probeHTTPAddressAllowed("127.0.0.1", netip.MustParseAddr("127.0.0.1")) ||
		!probeHTTPAddressAllowed("exporter.fixture", netip.MustParseAddr("10.10.0.20")) {
		t.Fatal("valid local exporter rejected")
	}
	for _, target := range []string{"http://fixture:secret@exporter.fixture/metrics", "http://exporter.fixture/metrics?token=secret",
		"http://exporter.fixture/metrics#fragment", "file:///tmp/metrics"} {
		obs := RunJob(context.Background(), openMetricsTestJob(target))
		if obs.Code != "invalid_openmetrics_config" || obs.Message != "" {
			t.Fatal("invalid URL reached network execution or reported raw configuration")
		}
	}
}

func TestOpenMetricsCollectorReadsBothTextProtocolsWithExactSelectors(t *testing.T) {
	for _, fixture := range []struct{ contentType, body string }{
		{"text/plain;version=0.0.4", "# TYPE requests_total counter\nrequests_total{route=\"a,b c\"} 0\nrequests_total{route=\"other-private\"} 9\nstale 4 1000\ninvalid NaN\n"},
		{"application/openmetrics-text;version=1.0.0", "# TYPE requests counter\nrequests_total{route=\"a,b c\"} 0\nrequests_total{route=\"other-private\"} 9\nstale 4 1.5\ninvalid +Inf\n# EOF\n"},
	} {
		t.Run(fixture.contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Path != "/metrics" || !strings.Contains(req.Header.Get("Accept"), "application/openmetrics-text") {
					t.Error("collector changed the assigned endpoint or request contract")
				}
				w.Header().Set("Content-Type", fixture.contentType)
				_, _ = w.Write([]byte(fixture.body))
			}))
			defer server.Close()
			obs := RunJob(context.Background(), openMetricsTestJob(server.URL+"/metrics"))
			if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "openmetrics" || obs.MonitoringSamples == nil {
				t.Fatalf("probe failed: %s", obs.Code)
			}
			samples := obs.MonitoringSamples.Samples
			if len(samples) != 3 || samples[0].Value == nil || *samples[0].Value != 0 ||
				samples[1].Gap != "missing" || samples[2].Gap != "missing" || len(monitoring.NumericProbeValues(obs.Payload, monitoring.OpenMetricsSource)) != 1 {
				t.Fatalf("exact selection, freshness or finite values lost: %+v", samples)
			}
		})
	}
}

func TestOpenMetricsCollectorRefusesInvalidMediaOversizeAndRedirect(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
	defer target.Close()
	for _, fixture := range []struct {
		name, content, body string
		status              int
	}{
		{"wrong_media", "application/json", "requests_total 2\n", 200},
		{"missing_eof", "application/openmetrics-text;version=1.0.0", "requests_total 2\n", 200},
		{"too_large", "text/plain", strings.Repeat("x", maxOpenMetricsBytes+1), 200},
		{"redirect", "text/plain", "", 302},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", fixture.content)
				w.Header().Set("Location", target.URL)
				w.WriteHeader(fixture.status)
				_, _ = w.Write([]byte(fixture.body))
			}))
			defer server.Close()
			obs := RunJob(context.Background(), openMetricsTestJob(server.URL))
			if obs.Status != "fail" || obs.ProbeReport == nil || obs.ProbeReport.Reason != "response" || len(obs.Payload) != 0 {
				t.Fatalf("invalid response became usable: %s", obs.Code)
			}
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect touched a different target")
	}
}

func TestOpenMetricsCollectorCancelsAnInFlightResponse(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan entities.AgentlessObservation, 1)
	go func() { result <- RunJob(ctx, openMetricsTestJob(server.URL)) }()
	select {
	case <-started:
	case obs := <-result:
		t.Fatalf("probe failed before opening the response: %s", obs.Code)
	}
	cancel()
	obs := <-result
	if obs.ProbeReport == nil || obs.ProbeReport.Outcome != "cancelled" {
		t.Fatal("interrupting the request did not cancel the probe")
	}
}

func openMetricsTestJob(target string) entities.AgentlessJob {
	return entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "openmetrics", BaseIntervalSec: 60,
		Endpoint:    &entities.AgentlessEndpoint{ID: 17, Tipo: "url", Endereco: target},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 1000, FreshnessSec: 180},
		Config: entities.AgentlessConfig{"metrics": []any{
			map[string]any{"id": "requests", "name": "requests_total", "labels": map[string]any{"route": "a,b c"}},
			map[string]any{"id": "stale", "name": "stale", "labels": map[string]any{}},
			map[string]any{"id": "invalid", "name": "invalid", "labels": map[string]any{}},
		}},
	}
}
