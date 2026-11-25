package agentless

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestHTTPObservationFeedsConfiguredMetricsWithoutAnotherRequest(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	job := canonicalTCPJob(0)
	job.Tipo = "http"
	job.Endpoint = &entities.AgentlessEndpoint{Tipo: "url", Endereco: server.URL}
	result := RunJob(context.Background(), job)
	if result.Status != "ok" || result.MonitoringSamples == nil || result.MonitoringSamples.Source != "agentless_http" {
		t.Fatal("HTTP collection did not produce the canonical latency")
	}
	qualifyLatencyMonitoringGraph(t, result, "http")
	if calls.Load() != 1 {
		t.Fatal("calculating dependent metrics repeated the HTTP request")
	}
}
