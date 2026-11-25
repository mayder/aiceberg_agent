package usecase

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestAgentlessCircuitCooldownAndRecoveryAreScoped(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	job := batchJob(1, "https://edge.fixture.example:8443/status")
	job.ClienteID, job.Tipo = 7, "http"
	key := agentlessCircuit(job, agentlessDestination(job))
	now := time.Unix(1700000000, 0)
	for i := 0; i < agentlessProbeFailureLimit; i++ {
		admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
		if i < agentlessProbeFailureLimit-1 && admission.circuitOpen(key, now) {
			t.Fatal("opened before the failure threshold")
		}
	}
	if !admission.circuitOpen(key, now) {
		t.Fatal("the offending endpoint was not guarded")
	}
	for _, independent := range []agentlessCircuitKey{
		{destination: agentlessDestinationKey{clientID: 8, host: key.destination.host}, protocol: "http", port: 8443},
		{destination: key.destination, protocol: "http", port: 443},
		{destination: key.destination, protocol: "tcp", port: 8443},
		{destination: agentlessDestinationKey{clientID: 7, host: "other.fixture.example"}, protocol: "http", port: 8443},
	} {
		if admission.circuitOpen(independent, now) {
			t.Fatal("open circuit leaked into an independent scope")
		}
	}
	now = now.Add(agentlessProbeCooldown)
	if admission.circuitOpen(key, now) {
		t.Fatal("half-open retry did not become eligible")
	}
	admission.recordCircuit(key, agentlessProbeIndeterminate, now)
	if !admission.circuitOpen(key, now) {
		t.Fatal("an inconclusive half-open attempt disabled the guard")
	}
	now = now.Add(agentlessProbeCooldown)
	admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	if !admission.circuitOpen(key, now) {
		t.Fatal("a failed half-open attempt did not extend the cooldown")
	}
	now = now.Add(agentlessProbeCooldown)
	admission.recordCircuit(key, agentlessProbeReachable, now)
	admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	if admission.circuitOpen(key, now) {
		t.Fatal("successful recovery failed to reset the failure count")
	}
	admission.prune(now.Add(agentlessProbeFailureTTL))
	if len(admission.circuits) != 0 {
		t.Fatal("stale circuit state was not removed")
	}
}

func TestAgentlessCircuitUsesNetworkEvidence(t *testing.T) {
	for _, fixture := range []struct {
		protocol, outcome, reason string
		want                      agentlessProbeOutcome
	}{
		{"http", "failed", "connection", agentlessProbeNetworkFailure},
		{"snmp", "timeout", "deadline", agentlessProbeNetworkFailure},
		{"tcp", "timeout", "budget", agentlessProbeBudgetExceeded},
		{"browser", "timeout", "deadline", agentlessProbeIndeterminate},
		{"browser", "timeout", "budget", agentlessProbeBudgetExceeded},
		{"discovery", "timeout", "budget", agentlessProbeBudgetExceeded},
		{"http", "cancelled", "cancelled", agentlessProbeIndeterminate},
		{"snmp", "failed", "configuration", agentlessProbeIndeterminate},
		{"http", "failed", "response", agentlessProbeIndeterminate},
		{"jmx", "unsupported", "protocol", agentlessProbeIndeterminate},
		{"tcp", "ok", "none", agentlessProbeReachable},
	} {
		report := &monitoring.ProbeReport{Protocol: fixture.protocol, Outcome: fixture.outcome, Reason: fixture.reason}
		if got := agentlessExecutionOutcome(entities.AgentlessObservation{ProbeReport: report}); got != fixture.want {
			t.Fatalf("unexpected result for %+v: %v", report, got)
		}
	}
}

func TestServerCancelledPostgresStatementProvesEndpointReachable(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	job := batchJob(6, "postgres.fixture.example")
	job.Tipo, job.ClienteID = "postgresql", 3
	key := agentlessCircuit(job, agentlessDestination(job))
	now := time.Unix(1700000000, 0)
	for range agentlessProbeFailureLimit {
		admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	}
	retry := now.Add(agentlessProbeCooldown)
	report := &monitoring.ProbeReport{Protocol: "postgresql", Outcome: "timeout", Reason: "deadline"}
	obs := entities.AgentlessObservation{Status: "fail", Code: "postgresql_statement_timeout", ProbeReport: report}
	admission.recordCircuit(key, agentlessExecutionOutcome(obs), retry)
	if _, tracked := admission.circuits[key]; tracked {
		t.Fatal("a server-cancelled SQL statement did not clear the connection circuit")
	}

	obs.Code = "postgresql_connection_failed"
	for range agentlessProbeFailureLimit {
		admission.recordCircuit(key, agentlessExecutionOutcome(obs), retry)
	}
	if !admission.circuitOpen(key, retry) {
		t.Fatal("PostgreSQL connection timeouts no longer trip the circuit")
	}
}

func TestServiceAuthenticationRejectionDoesNotPauseNeighboringCheck(t *testing.T) {
	for protocol, code := range map[string]string{
		"postgresql": "postgresql_access_denied", "mysql": "mysql_access_denied", "vcenter": "vcenter_access_denied",
		"jolokia": "http_access_denied", "openmetrics": "http_access_denied", "ssh": "ssh_access_denied",
	} {
		t.Run(protocol, func(t *testing.T) {
			failing := batchJob(1, "service.fixture.example")
			failing.Tipo, failing.ClienteID = protocol, 7
			neighbor := failing
			neighbor.CheckID = 2
			destination := agentlessDestination(failing)
			key := agentlessCircuit(failing, destination)
			now := time.Unix(1700000000, 0)
			admission := newAgentlessProbeAdmission()
			for range agentlessProbeFailureLimit {
				admission.recordExecution(failing, destination, agentlessProbeNetworkFailure, now)
			}
			if !admission.circuitOpen(key, now) {
				t.Fatal("network failures did not open the service circuit")
			}
			retry := now.Add(agentlessProbeCooldown)
			obs := entities.AgentlessObservation{Status: "fail", Code: code,
				ProbeReport: &monitoring.ProbeReport{Protocol: protocol, Outcome: "failed", Reason: "configuration"}}
			for range agentlessProbeFailureLimit {
				admission.recordExecution(failing, destination, agentlessExecutionOutcome(obs), retry)
			}
			schedule := newAgentlessBatchSchedule([]entities.AgentlessJob{neighbor}, 1, 1)
			schedule.admission = admission
			if work, ok := schedule.take(retry); !ok || work.job.CheckID != neighbor.CheckID {
				t.Fatal("authentication failure kept the neighbor's service circuit open")
			}
		})
	}
}

func TestJMXBudgetExhaustionPausesOnlyTheFailingCheck(t *testing.T) {
	slow := batchJob(1, "jmx.fixture.example")
	slow.Tipo = "jmx"
	neighbor := slow
	neighbor.CheckID++
	admission := newAgentlessProbeAdmission()
	now := time.Unix(1700000000, 0)
	report := &monitoring.ProbeReport{Protocol: "jmx", Outcome: "timeout", Reason: "budget"}
	for range agentlessProbeFailureLimit {
		admission.recordExecution(slow, agentlessDestination(slow),
			agentlessExecutionOutcome(entities.AgentlessObservation{Code: "jmx_timeout", ProbeReport: report}), now)
	}
	schedule := newAgentlessBatchSchedule([]entities.AgentlessJob{slow, neighbor}, 2, 2)
	schedule.admission = admission
	work, ok := schedule.take(now)
	if !ok || work.job.CheckID != neighbor.CheckID || schedule.budgetDeferred != 1 || schedule.circuitDeferred != 0 {
		t.Fatal("JMX timeout blocked checks sharing the same service")
	}
}

func TestAgentlessPacingReusesStateAcrossBatches(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	first := batchJob(1, "http://127.0.0.1:19001/status")
	second := batchJob(2, "127.0.0.1:19002")
	first.ClienteID, second.ClienteID = 3, 3
	policy := agentlessBatchPolicy{workers: 2, limit: 1, admission: admission}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var started []time.Time
	run := func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
		started = append(started, time.Now())
		return agentlessProbeReachable, nil
	}
	for _, job := range []entities.AgentlessJob{first, second} {
		if stats, err := runAgentlessBatch(ctx, []entities.AgentlessJob{job}, policy, run); err != nil || stats.dispatched != 1 {
			t.Fatalf("paced work did not run: %+v %v", stats, err)
		}
	}
	if elapsed := started[1].Sub(started[0]); elapsed < agentlessProbeSpacing {
		t.Fatalf("a second protocol or port bypassed pacing: %s", elapsed)
	}
}

func TestAgentlessPacingLeavesWorkersAvailableAndCanCancel(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	paced, free := batchJob(1, "slow.fixture.example"), batchJob(2, "free.fixture.example")
	policy := agentlessBatchPolicy{workers: 1, limit: 2, admission: admission}
	seedAgentlessPacing(t, admission, agentlessDestination(paced), time.Now().Add(time.Minute))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started []int
	stats, err := runAgentlessBatch(ctx, []entities.AgentlessJob{paced, free}, policy, func(_ context.Context, j entities.AgentlessJob) (agentlessProbeOutcome, error) {
		started = append(started, j.CheckID)
		cancel()
		return agentlessProbeReachable, nil
	})
	if !errors.Is(err, context.Canceled) || stats.dispatched != 1 || len(started) != 1 || started[0] != free.CheckID {
		t.Fatalf("paced endpoint occupied the worker or ran after cancellation: %+v %v %v", stats, started, err)
	}
}

func TestAgentlessHubCircuitDefersOnlyTheFailingService(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("fixture"))
	}))
	defer healthy.Close()
	closed, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	unavailable := "http://" + closed.Addr().String()
	_ = closed.Close()
	offline := entities.AgentlessJob{CheckID: 71, AtivoID: 5, ClienteID: 3, Tipo: "http",
		BaseIntervalSec: 60, TimeoutMs: 1000, Endpoint: &entities.AgentlessEndpoint{ID: 1, Tipo: "url", Endereco: unavailable}}
	online := offline
	online.CheckID = 72
	online.Endpoint = &entities.AgentlessEndpoint{ID: 2, Tipo: "url", Endereco: healthy.URL}
	assertAgentlessCircuitDefersOnlyTheFailingCheck(t, offline, online)
}

func TestAgentlessHubCircuitUsesConfiguredTCPPorts(t *testing.T) {
	healthy, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer healthy.Close()
	closed, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	unavailable := closed.Addr().(*net.TCPAddr).Port
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	offline := entities.AgentlessJob{CheckID: 71, AtivoID: 5, ClienteID: 3, Tipo: "tcp", BaseIntervalSec: 60, TimeoutMs: 1000,
		Endpoint: &entities.AgentlessEndpoint{ID: 2, Tipo: "ip", Endereco: "127.0.0.1"},
		Config:   entities.AgentlessConfig{"port": strconv.Itoa(unavailable)}}
	online := offline
	online.CheckID = 72
	online.Config = entities.AgentlessConfig{"port": float64(healthy.Addr().(*net.TCPAddr).Port)}
	assertAgentlessCircuitDefersOnlyTheFailingCheck(t, offline, online)
}

func TestAgentlessCircuitMatchesConfiguredHTTPAndTLSTransport(t *testing.T) {
	port := 9443
	for _, fixture := range []struct {
		protocol string
		endpoint entities.AgentlessEndpoint
		config   entities.AgentlessConfig
		want     int
	}{
		{"http", entities.AgentlessEndpoint{Tipo: "ip", Endereco: "127.0.0.1"}, entities.AgentlessConfig{"scheme": "https"}, 443},
		{"http", entities.AgentlessEndpoint{Tipo: "ip", Endereco: "127.0.0.1"}, entities.AgentlessConfig{"https": true, "port": "8443"}, 8443},
		{"http", entities.AgentlessEndpoint{Tipo: "url", Endereco: "127.0.0.1:8080/status", Porta: &port}, nil, 8080},
		{"http", entities.AgentlessEndpoint{Tipo: "url", Endereco: "https://127.0.0.1/status", Porta: &port}, nil, 443},
		{"tls", entities.AgentlessEndpoint{Tipo: "ip", Endereco: "127.0.0.1"}, entities.AgentlessConfig{"port": 8443}, 8443},
		{"tls", entities.AgentlessEndpoint{Tipo: "ip", Endereco: "127.0.0.1", Porta: &port}, entities.AgentlessConfig{"port": 8443}, 9443},
	} {
		job := entities.AgentlessJob{Tipo: fixture.protocol, Endpoint: &fixture.endpoint, Config: fixture.config}
		if reported := agentlessCircuit(job, agentlessDestination(job)).port; reported != fixture.want {
			t.Errorf("%s %s: circuit port %d, want %d", fixture.protocol, fixture.endpoint.Endereco, reported, fixture.want)
		}
	}
}

func TestAgentlessCircuitAndRateUnifyEquivalentHTTPAddresses(t *testing.T) {
	explicit := batchJob(1, "http://EDGE.fixture.example:8080/status")
	explicit.Tipo = "http"
	explicit.Endpoint.Tipo = "url"
	implicit := explicit
	implicit.CheckID = 2
	implicit.Endpoint = &entities.AgentlessEndpoint{ID: 2, Tipo: "url", Endereco: "edge.fixture.example:8080/info"}
	otherPort := implicit
	otherPort.Endpoint = &entities.AgentlessEndpoint{ID: 3, Tipo: "url", Endereco: "edge.fixture.example:8081/info"}
	key := agentlessDestination(explicit)
	if agentlessDestination(implicit) != key || agentlessDestination(otherPort) != key {
		t.Fatal("equivalent hosts must share the rate limit across ports")
	}
	circuit := agentlessCircuit(explicit, key)
	if agentlessCircuit(implicit, key) != circuit || agentlessCircuit(otherPort, key) == circuit {
		t.Fatal("the service circuit must distinguish ports, independent of URL spelling")
	}
}

func assertAgentlessCircuitDefersOnlyTheFailingCheck(t *testing.T, offline, online entities.AgentlessJob) {
	t.Helper()
	outbox := &batchOutbox{}
	uc := NewAgentlessHub(config.Config{}, testLogger{}, nil, outbox, nil, nil)
	uc.targets = []entities.AgentlessJob{offline, online}
	for range agentlessProbeFailureLimit + 1 {
		if err := uc.PollAndRun(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	failures, successes := 0, 0
	for _, observation := range outbox.observations {
		switch {
		case observation.CheckID == offline.CheckID && observation.Status == "fail":
			failures++
		case observation.CheckID == online.CheckID && observation.Status == "ok":
			successes++
		default:
			t.Fatalf("unexpected health observation: %+v", observation)
		}
	}
	if failures != agentlessProbeFailureLimit || successes != agentlessProbeFailureLimit+1 {
		t.Fatalf("expected open circuit only on failing port: failures=%d successes=%d", failures, successes)
	}
}

func TestAgentlessManualProbeDoesNotResetTheRoutineCircuit(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	job := batchJob(7, "fixture.example")
	job.ClienteID, job.Tipo = 3, "snmp"
	key := agentlessCircuit(job, agentlessDestination(job))
	now := time.Now()
	for range agentlessProbeFailureLimit {
		admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	}
	var mu sync.Mutex
	var runs int
	run := func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		return agentlessProbeReachable, nil
	}
	policy := agentlessBatchPolicy{workers: 1, limit: 1, admission: admission, manual: true}
	if stats, err := runAgentlessBatch(context.Background(), []entities.AgentlessJob{job}, policy, run); err != nil || stats.dispatched != 1 {
		t.Fatalf("manual diagnostics were blocked: %+v %v", stats, err)
	}
	policy.manual = false
	if stats, err := runAgentlessBatch(context.Background(), []entities.AgentlessJob{job}, policy, run); err != nil || stats.circuitDeferred != 1 || runs != 1 {
		t.Fatalf("manual result opened the automatic queue: %+v %v runs=%d", stats, err, runs)
	}
}

func TestAgentlessHalfOpenAdmitsOneProbeBeforeReopening(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	first := batchJob(1, "http://fixture.example:8080/")
	first.ClienteID, first.Tipo = 3, "http"
	second := first
	second.CheckID++
	key := agentlessCircuit(first, agentlessDestination(first))
	now := time.Now().Add(-agentlessProbeCooldown)
	for range agentlessProbeFailureLimit {
		admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	}
	policy := agentlessBatchPolicy{workers: 2, limit: 2, admission: admission}
	stats, err := runAgentlessBatch(context.Background(), []entities.AgentlessJob{first, second}, policy,
		func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
			return agentlessProbeNetworkFailure, nil
		})
	if err != nil || stats.dispatched != 1 || stats.circuitDeferred != 1 {
		t.Fatalf("half-open admitted more than one probe: %+v %v", stats, err)
	}
}

func TestAgentlessPacingWaitIsInterruptibleWithoutAnActiveWorker(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	job := batchJob(1, "fixture.example")
	seedAgentlessPacing(t, admission, agentlessDestination(job), time.Now().Add(time.Minute))
	policy := agentlessBatchPolicy{workers: 1, limit: 1, admission: admission}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	stats, err := runAgentlessBatch(ctx, []entities.AgentlessJob{job}, policy,
		func(context.Context, entities.AgentlessJob) (agentlessProbeOutcome, error) {
			t.Error("paced work opened a probe after its outer deadline")
			return agentlessProbeIndeterminate, nil
		})
	if !errors.Is(err, context.DeadlineExceeded) || stats.dispatched != 0 {
		t.Fatalf("pacing wait failed to stop: %+v %v", stats, err)
	}
}

func TestAgentlessCircuitCapacityDoesNotLetOneClientFillAllState(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	now := time.Now()
	for i := 0; i <= agentlessCircuitKeysPerClient; i++ {
		key := agentlessCircuitKey{destination: agentlessDestinationKey{clientID: 1, host: fmt.Sprintf("hot-%d", i)}, protocol: "tcp", port: 443}
		admission.recordCircuit(key, agentlessProbeNetworkFailure, now)
	}
	independent := agentlessCircuitKey{destination: agentlessDestinationKey{clientID: 2, host: "cool"}, protocol: "tcp", port: 443}
	for range agentlessProbeFailureLimit {
		admission.recordCircuit(independent, agentlessProbeNetworkFailure, now)
	}
	if len(admission.circuits) != agentlessCircuitKeysPerClient+1 || !admission.circuitOpen(independent, now) {
		t.Fatal("one client consumed or disabled independent circuit state")
	}
	admission.prune(now.Add(agentlessProbeFailureTTL))
	if len(admission.circuitsByClient) != 0 || len(admission.circuits) != 0 {
		t.Fatal("stale entries held client capacity")
	}
}

func TestAgentlessRateCapacityKeepsOtherClientsRunnable(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	now := time.Unix(1700000000, 0)
	for i := range agentlessRateKeysPerClient {
		key := agentlessDestinationKey{clientID: 1, host: fmt.Sprintf("fixture-%d", i)}
		seedAgentlessPacing(t, admission, key, now)
	}
	saturated := batchJob(1, "shared.fixture.example")
	independent := batchJob(2, "shared.fixture.example")
	saturated.ClienteID, independent.ClienteID = 1, 2
	schedule := newAgentlessBatchSchedule([]entities.AgentlessJob{saturated, independent}, 2, 2)
	schedule.admission = admission

	work, ok := schedule.take(now)
	if !ok || work.job.CheckID != independent.CheckID || !schedule.wakeAt.Equal(now.Add(agentlessProbeSpacing)) {
		t.Fatal("one client at its pacing limit held another client's worker")
	}
	// Dispatch was cancelled before the worker started.
	schedule.complete(agentlessBatchResult{work: work})
	if admission.ratesByClient[2] != 0 {
		t.Fatal("a cancelled dispatch leaked a pacing slot")
	}
	work, ok = schedule.take(now.Add(agentlessProbeSpacing))
	if !ok || work.job.CheckID != saturated.CheckID || admission.ratesByClient[1] != 1 {
		t.Fatal("the saturated client failed to resume after its pacing window")
	}
}

func TestAgentlessGlobalRateCapacityResumesAtFirstVacancy(t *testing.T) {
	admission := newAgentlessProbeAdmission()
	now := time.Unix(1700000000, 0)
	for i := range agentlessAdmissionMaxKeys {
		key := agentlessDestinationKey{clientID: 1 + i/agentlessRateKeysPerClient, host: fmt.Sprintf("fixture-%d", i)}
		started := now.Add(10 * time.Millisecond)
		if i == 0 {
			started = now
		}
		seedAgentlessPacing(t, admission, key, started)
	}
	newClient := agentlessDestinationKey{clientID: 100, host: "independent.fixture.example"}
	due := now.Add(agentlessProbeSpacing)
	if ready, next := admission.reserveRate(newClient, now.Add(10*time.Millisecond)); ready || !next.Equal(due) {
		t.Fatalf("a full HUB should wait only until the first capacity slot expires: ready=%t next=%v", ready, next)
	}
	if ready, _ := admission.reserveRate(newClient, due); !ready || len(admission.nextStart) != agentlessAdmissionMaxKeys {
		t.Fatal("the HUB failed to reuse the first expired pacing slot")
	}
}

func seedAgentlessPacing(t *testing.T, admission *agentlessProbeAdmission, key agentlessDestinationKey, started time.Time) {
	t.Helper()
	if ready, _ := admission.reserveRate(key, started); !ready {
		t.Fatal("test pacing slot was rejected while seeding")
	}
	admission.finishRate(key, started, started)
}
