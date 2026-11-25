package usecase

import (
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

const (
	agentlessProbeSpacing         = 100 * time.Millisecond
	agentlessProbeFailureLimit    = 3
	agentlessProbeCooldown        = 30 * time.Second
	agentlessProbeFailureTTL      = 5 * time.Minute
	agentlessAdmissionMaxKeys     = 4096
	agentlessCircuitKeysPerClient = 512
)

type agentlessCircuitKey struct {
	destination agentlessDestinationKey
	protocol    string
	port        int
	checkID     int // Zero identifies the service circuit.
}

type agentlessCircuitState struct {
	failures int
	retryAt  time.Time
	seenAt   time.Time
}

type agentlessProbeOutcome uint8

const (
	agentlessProbeIndeterminate agentlessProbeOutcome = iota
	agentlessProbeReachable
	agentlessProbeNetworkFailure
	agentlessProbeBudgetExceeded
	agentlessProbeInterrupted
)

// The batch dispatcher owns this state. The HUB's run slot serializes batches.
type agentlessProbeAdmission struct {
	nextStart        map[agentlessDestinationKey]time.Time
	ratesByClient    map[int]int
	circuits         map[agentlessCircuitKey]agentlessCircuitState
	circuitsByClient map[int]int
}

func newAgentlessProbeAdmission() *agentlessProbeAdmission {
	return &agentlessProbeAdmission{
		nextStart:        make(map[agentlessDestinationKey]time.Time),
		ratesByClient:    make(map[int]int),
		circuits:         make(map[agentlessCircuitKey]agentlessCircuitState),
		circuitsByClient: make(map[int]int),
	}
}

func (p *agentlessProbeAdmission) prune(now time.Time) {
	if p == nil {
		return
	}
	p.pruneRates(now)
	for key, state := range p.circuits {
		if now.Sub(state.seenAt) >= agentlessProbeFailureTTL {
			p.removeCircuit(key)
		}
	}
}

func (p *agentlessProbeAdmission) circuitOpen(key agentlessCircuitKey, now time.Time) bool {
	if p == nil {
		return false
	}
	return now.Before(p.circuits[key].retryAt)
}

func (p *agentlessProbeAdmission) recordCircuit(key agentlessCircuitKey, outcome agentlessProbeOutcome, now time.Time) {
	if p == nil {
		return
	}
	if outcome == agentlessProbeReachable {
		p.removeCircuit(key)
		return
	}
	state, tracked := p.circuits[key]
	if tracked && now.Sub(state.seenAt) >= agentlessProbeFailureTTL {
		p.removeCircuit(key)
		state, tracked = agentlessCircuitState{}, false
	}
	if outcome == agentlessProbeIndeterminate {
		if tracked && state.failures >= agentlessProbeFailureLimit {
			// A half-open probe with inconclusive evidence keeps the circuit guarded.
			state.retryAt = now.Add(agentlessProbeCooldown)
			p.circuits[key] = state
		}
		return
	}
	if !tracked {
		clientID := key.destination.clientID
		if p.circuitCapacityExceeded(clientID) {
			p.prune(now)
			if p.circuitCapacityExceeded(clientID) {
				return
			}
		}
		p.circuitsByClient[clientID]++
	}
	state.failures = min(state.failures+1, agentlessProbeFailureLimit)
	state.seenAt = now
	if state.failures >= agentlessProbeFailureLimit {
		state.retryAt = now.Add(agentlessProbeCooldown)
	}
	p.circuits[key] = state
}

func (p *agentlessProbeAdmission) recordExecution(job entities.AgentlessJob, destination agentlessDestinationKey, outcome agentlessProbeOutcome, now time.Time) {
	if outcome == agentlessProbeInterrupted {
		return
	}
	service := agentlessCircuit(job, destination)
	check := service
	check.checkID = job.CheckID
	if outcome == agentlessProbeBudgetExceeded {
		if check.checkID > 0 {
			p.recordCircuit(check, outcome, now)
		}
		p.recordCircuit(service, agentlessProbeIndeterminate, now)
		return
	}
	p.recordCircuit(service, outcome, now)
	if check.checkID > 0 {
		// A classified network failure completed without exhausting the check budget.
		if outcome == agentlessProbeNetworkFailure {
			outcome = agentlessProbeReachable
		}
		p.recordCircuit(check, outcome, now)
	}
}

func (p *agentlessProbeAdmission) circuitCapacityExceeded(clientID int) bool {
	return len(p.circuits) >= agentlessAdmissionMaxKeys || p.circuitsByClient[clientID] >= agentlessCircuitKeysPerClient
}

func (p *agentlessProbeAdmission) removeCircuit(key agentlessCircuitKey) {
	if _, exists := p.circuits[key]; !exists {
		return
	}
	delete(p.circuits, key)
	clientID := key.destination.clientID
	p.circuitsByClient[clientID]--
	if p.circuitsByClient[clientID] == 0 {
		delete(p.circuitsByClient, clientID)
	}
}

func agentlessCircuit(job entities.AgentlessJob, destination agentlessDestinationKey) agentlessCircuitKey {
	protocol := strings.ToLower(job.Tipo)
	if _, port, handled := agentless.ActiveProbeTarget(job); handled {
		return agentlessCircuitKey{destination: destination, protocol: protocol, port: port}
	}
	port := agentlessDefaultPort(protocol)
	if job.Endpoint != nil {
		if endpoint, err := url.Parse(job.Endpoint.Endereco); err == nil && endpoint.Host != "" {
			switch endpoint.Scheme {
			case "https":
				port = 443
			case "http":
				port = 80
			}
			if explicit, err := strconv.Atoi(endpoint.Port()); err == nil && explicit > 0 {
				port = explicit
			}
		} else if job.Endpoint.Porta != nil && *job.Endpoint.Porta > 0 {
			port = *job.Endpoint.Porta
		} else if _, rawPort, err := net.SplitHostPort(job.Endpoint.Endereco); err == nil {
			if explicit, err := strconv.Atoi(rawPort); err == nil && explicit > 0 {
				port = explicit
			}
		}
	}
	if protocol == "snmp" && job.SNMP != nil && job.SNMP.Port > 0 {
		port = job.SNMP.Port
	}
	return agentlessCircuitKey{destination: destination, protocol: protocol, port: port}
}

func agentlessDefaultPort(protocol string) int {
	switch protocol {
	case "openmetrics", "jolokia":
		return 80
	case "https", "vcenter", "browser":
		return 443
	case "snmp":
		return 161
	case "dns":
		return 53
	case "ssh":
		return 22
	case "telnet":
		return 23
	case "ipmi":
		return 623
	case "mysql":
		return 3306
	case "postgresql":
		return 5432
	default:
		return 0
	}
}

func agentlessExecutionOutcome(observation entities.AgentlessObservation) agentlessProbeOutcome {
	report := observation.ProbeReport
	if report == nil {
		return agentlessProbeIndeterminate
	}
	switch {
	case report.Outcome == "failed" && report.Reason == "configuration" &&
		serviceAuthRejected(report.Protocol, observation.Code):
		// An explicit service authentication reply proves network reachability.
		return agentlessProbeReachable
	case report.Outcome == "timeout" && report.Protocol == "postgresql" && observation.Code == "postgresql_statement_timeout":
		// SQLSTATE 57014 is a reply from the authenticated PostgreSQL session.
		return agentlessProbeReachable
	case report.Outcome == "timeout" && report.Reason == "budget":
		return agentlessProbeBudgetExceeded
	case report.Outcome == "ok" && report.Protocol == "snmp" && observation.Payload["time_budget_exceeded"] == true:
		// Preserve the usable SNMP observation while pacing the incomplete check.
		return agentlessProbeBudgetExceeded
	case report.Outcome == "ok":
		return agentlessProbeReachable
	case report.Outcome == "failed" && report.Reason == "connection":
		return agentlessProbeNetworkFailure
	case report.Outcome == "timeout" && report.Protocol != "browser" && report.Protocol != "discovery":
		return agentlessProbeNetworkFailure
	default:
		return agentlessProbeIndeterminate
	}
}

func serviceAuthRejected(protocol, code string) bool {
	switch protocol {
	case "postgresql", "mysql", "vcenter", "ssh":
		return code == protocol+"_access_denied"
	case "jolokia", "openmetrics":
		return code == "http_access_denied"
	default:
		return false
	}
}
