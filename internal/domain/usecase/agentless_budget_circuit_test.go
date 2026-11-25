package usecase

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestAgentlessHubPausesOnlyTheCheckExhaustingBudget(t *testing.T) {
	var slowCalls, healthyCalls atomic.Int32
	var recovered atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/slow" {
			slowCalls.Add(1)
			if !recovered.Load() {
				<-req.Context().Done()
				return
			}
		} else {
			healthyCalls.Add(1)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	check := entities.AgentlessJob{CheckID: 21, AtivoID: 3, ClienteID: 9, Tipo: "http", BaseIntervalSec: 60,
		Endpoint:    &entities.AgentlessEndpoint{ID: 5, Tipo: "url", Endereco: server.URL + "/slow"},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 120, FreshnessSec: 180}}
	healthy := check
	healthy.CheckID = 22
	healthy.Endpoint = &entities.AgentlessEndpoint{ID: 5, Tipo: "url", Endereco: server.URL + "/healthy"}
	outbox := &batchOutbox{}
	log := &fakeLogger{}
	hub := NewAgentlessHub(config.Config{}, log, nil, outbox, nil, nil)
	hub.setTargets([]entities.AgentlessJob{check, healthy})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for range agentlessProbeFailureLimit + 1 {
		if err := hub.PollAndRun(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if slowCalls.Load() != 3 || healthyCalls.Load() != 4 || len(outbox.observations) != 7 {
		t.Fatalf("unexpected pause behavior: slow=%d healthy=%d observations=%d", slowCalls.Load(), healthyCalls.Load(), len(outbox.observations))
	}
	if !strings.Contains(strings.Join(log.info, "\n"), "budget_deferred=1") {
		t.Fatal("the budget pause did not record its deferral")
	}
	assertCheckBudgetPause(t, hub, check, &recovered)
}

func assertCheckBudgetPause(t *testing.T, hub *AgentlessHub, check entities.AgentlessJob, recovered *atomic.Bool) {
	t.Helper()
	now := time.Now()
	service := agentlessCircuit(check, agentlessDestination(check))
	checkCircuit := service
	checkCircuit.checkID = check.CheckID
	if hub.probeAdmission.circuitOpen(service, now) || !hub.probeAdmission.circuitOpen(checkCircuit, now) {
		t.Fatal("budget exhaustion opened the wrong circuit")
	}

	recovered.Store(true)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hub.setTargets([]entities.AgentlessJob{check})
	if err := hub.runJobs(ctx, []entities.AgentlessJob{check}, agentlessBatchPolicy{limit: 1, manual: true}); err != nil {
		t.Fatal(err)
	}
	if !hub.probeAdmission.circuitOpen(checkCircuit, time.Now()) {
		t.Fatal("manual diagnosis changed the routine's budget pause")
	}
	recovered.Store(false)
	state := hub.probeAdmission.circuits[checkCircuit]
	state.retryAt = time.Now()
	hub.probeAdmission.circuits[checkCircuit] = state
	if err := hub.PollAndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if !hub.probeAdmission.circuitOpen(checkCircuit, time.Now()) {
		t.Fatal("a repeated overrun did not reopen the pause on the first retry")
	}
	recovered.Store(true)
	state = hub.probeAdmission.circuits[checkCircuit]
	state.retryAt = time.Now()
	hub.probeAdmission.circuits[checkCircuit] = state
	if err := hub.PollAndRun(ctx); err != nil {
		t.Fatal(err)
	}
	if _, tracked := hub.probeAdmission.circuits[checkCircuit]; tracked {
		t.Fatal("successful routine retry did not clear its budget pause")
	}
}

func TestAgentlessSNMPUsablePartialResultStillCountsBudgetExhaustion(t *testing.T) {
	obs := entities.AgentlessObservation{
		Status:      "ok",
		ProbeReport: &monitoring.ProbeReport{Outcome: "ok", Protocol: "snmp", Reason: "none"},
		Payload:     map[string]any{"time_budget_exceeded": true},
	}
	if outcome := agentlessExecutionOutcome(obs); outcome != agentlessProbeBudgetExceeded {
		t.Fatal("the usable SNMP data hid a budget overrun")
	}
}
