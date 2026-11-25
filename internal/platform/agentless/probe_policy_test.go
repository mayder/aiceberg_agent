package agentless

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestProbePolicyBoundsHTTPAndRejectsInvalidPolicyBeforeCollection(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	defer server.Close()
	job := entities.AgentlessJob{ClienteID: 2, AtivoID: 52, CheckID: 73, Tipo: "http", IntervalSec: 60, TimeoutMs: 5000,
		Endpoint: &entities.AgentlessEndpoint{Tipo: "url", Endereco: server.URL}}
	job.ProbePolicy = &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 180, FreshnessSec: 180}
	obs := RunJob(context.Background(), job)
	if requests.Load() != 1 || obs.Status != "fail" || obs.ProbeReport == nil || obs.ProbeReport.Outcome != "timeout" ||
		obs.ProbeReport.Reason != "budget" || obs.ProbeReport.ElapsedMS > 2000 {
		t.Fatalf("HTTP job did not follow the bounded policy: requests=%d report=%+v legacy_code=%s", requests.Load(), obs.ProbeReport, obs.Code)
	}

	job.ProbePolicy.FreshnessSec = 900
	obs = RunJob(context.Background(), job)
	if requests.Load() != 1 || obs.Code != "invalid_probe_policy" || obs.ProbeReport == nil || obs.ProbeReport.Reason != "configuration" {
		t.Fatal("invalid job reached its adapter")
	}
}

func TestSNMPProbePolicyUsesResolvedProfileAndBaseFreshness(t *testing.T) {
	job := entities.AgentlessJob{ClienteID: 2, AtivoID: 52, CheckID: 73, Tipo: "snmp", IntervalSec: 300, BaseIntervalSec: 60,
		SNMP: &entities.AgentlessSnmpProfile{ProfileID: 41}, ProbePolicy: &monitoring.ProbePolicy{
			Contract: monitoring.ProbePolicyContract, FreshnessSec: 180, CredentialRef: "snmp_profile:41",
		}}
	parent, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	ctx, cancel, err := probeExecutionContext(parent, job, "snmp")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if ctx != parent || parent.Err() != nil {
		t.Fatal("zero extra budget must leave the adapter and caller deadline intact")
	}
	job.ProbePolicy.CredentialRef = "snmp_profile:42"
	obs := RunJob(parent, job)
	if obs.Code != "invalid_probe_policy" {
		t.Fatal("different profile reached the SNMP adapter", obs.Code)
	}
}
