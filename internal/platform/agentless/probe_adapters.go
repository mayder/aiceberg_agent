package agentless

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

var errProbeBudget = errors.New("monitoring_probe_budget_exhausted")

// Adapter output is independent of protocol-specific clients and wire formats.
type probeSegment struct {
	Group  string
	Result Result
}

type probeSegmentSink func(probeSegment)

type probeAdapter struct {
	collect       func(context.Context, entities.AgentlessJob, probeSegmentSink) Result
	flushPartials func(entities.AgentlessJob) bool
	cost          func(entities.AgentlessJob) int
	failures      map[string]probeClassification
}

type SNMPCredentialResolver func(context.Context, entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error)

// ObservationCost bounds the outbox entries a probe can emit in one execution.
func ObservationCost(job entities.AgentlessJob) int {
	if cost := probeAdapters[probeProtocol(job.Tipo)].cost; cost != nil {
		return cost(job)
	}
	return 1
}

func snmpObservationCost(job entities.AgentlessJob) int {
	if job.SNMP == nil ||
		job.Config["snmp_basic_probe"] == true || !shouldFlushSNMPPartials(job) || endpointHost(job) == "" {
		return 1
	}
	plan := buildSNMPPlan(job, "")
	cost := 1
	for _, name := range plan.Groups {
		if _, supported := snmpGroupDefs[name]; supported {
			cost++
		}
	}
	if len(plan.CustomGet)+len(plan.CustomWalk) > 0 {
		cost++
	}
	return cost
}

func RunJob(ctx context.Context, job entities.AgentlessJob) entities.AgentlessObservation {
	return RunJobWithPartials(ctx, job, nil)
}

func RunJobWithPartials(ctx context.Context, job entities.AgentlessJob, onPartial func(entities.AgentlessObservation)) entities.AgentlessObservation {
	return RunJobWithCredentials(ctx, job, onPartial, nil)
}

func RunJobWithCredentials(ctx context.Context, job entities.AgentlessJob, onPartial func(entities.AgentlessObservation),
	resolve SNMPCredentialResolver,
) entities.AgentlessObservation {
	protocol := probeProtocol(job.Tipo)
	started := time.Now()
	adapter, supported := probeAdapters[protocol]
	if !supported {
		res := Result{Status: "fail", Code: "unknown_type", Message: "tipo nao suportado"}
		return withProbeReport(ctx, job, newObservationFromResult(job, res, started), "unsupported", started, res.failure)
	}
	if ctx.Err() != nil {
		return interruptedProbe(ctx, job, protocol, started)
	}
	probeCtx, cancel, err := probeExecutionContext(ctx, job, protocol)
	if err != nil {
		if protocol == "snmp" && job.CredentialDelivery != "" {
			return unavailableProbeCredential(ctx, job, protocol, started, err)
		}
		res := Result{Status: "fail", Code: "invalid_probe_policy"}
		return withProbeReport(ctx, job, newObservationFromResult(job, res, started), protocol, started, nil)
	}
	defer cancel()
	ctx = probeCtx
	if job.CredentialDelivery != "" {
		profile, err := resolveProbeCredential(ctx, job, protocol, resolve)
		if err != nil {
			if ctx.Err() != nil {
				return interruptedProbe(ctx, job, protocol, started)
			}
			return unavailableProbeCredential(ctx, job, protocol, started, err)
		}
		job.SNMP = profile
	}
	return runProbeAdapter(ctx, job, protocol, adapter, started, onPartial)
}

func probeProtocol(kind string) string {
	protocol := strings.ToLower(kind)
	if protocol == "discovery_assisted" {
		return "discovery"
	}
	return protocol
}

func unavailableProbeCredential(ctx context.Context, job entities.AgentlessJob, protocol string, started time.Time, err error) entities.AgentlessObservation {
	res := Result{Status: "fail", Code: "snmp_credential_unavailable"}
	obs := newObservationFromResult(job, res, started)
	if protocol == "snmp" {
		applyObservationSegmentState(&obs, 1, false, true, "final")
	}
	return withProbeReport(ctx, job, obs, protocol, started, err)
}

func resolveProbeCredential(ctx context.Context, job entities.AgentlessJob, protocol string,
	resolve SNMPCredentialResolver,
) (*entities.AgentlessSnmpProfile, error) {
	if protocol != "snmp" || job.CredentialDelivery != entities.AgentlessCredentialReferenceV1 ||
		job.ProbePolicy == nil || job.SNMP == nil || job.Endpoint == nil || resolve == nil {
		return nil, monitoring.ErrProbePolicy
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	profile, err := resolve(ctx, job)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.WithoutCredential() != job.SNMP.WithoutCredential() {
		return nil, monitoring.ErrProbePolicy
	}
	return profile, ctx.Err()
}

func probeExecutionContext(ctx context.Context, job entities.AgentlessJob, protocol string) (context.Context, context.CancelFunc, error) {
	p := job.ProbePolicy
	if p == nil {
		return ctx, func() {}, nil
	}
	var profileID int64
	if protocol == "snmp" && job.SNMP != nil {
		profileID = int64(job.SNMP.ProfileID)
	}
	if err := p.Validate(protocol, profileID); err != nil {
		return nil, nil, err
	}
	if p.FreshnessSec != jobProbeFreshness(job) {
		return nil, nil, monitoring.ErrProbePolicy
	}
	if p.BudgetMS == 0 {
		return ctx, func() {}, nil
	}
	bounded, cancel := context.WithTimeoutCause(ctx, time.Duration(p.BudgetMS)*time.Millisecond, errProbeBudget)
	return bounded, cancel, nil
}

func runProbeAdapter(ctx context.Context, job entities.AgentlessJob, protocol string, adapter probeAdapter, started time.Time,
	onPartial func(entities.AgentlessObservation),
) entities.AgentlessObservation {
	if ctx.Err() != nil {
		return interruptedProbe(ctx, job, protocol, started)
	}
	seq := 0
	var sink probeSegmentSink
	if onPartial != nil && adapter.flushPartials != nil && adapter.flushPartials(job) {
		sink = func(partial probeSegment) {
			if ctx.Err() != nil {
				return
			}
			seq++
			obs := newObservationFromResult(job, partial.Result, started)
			applyObservationSegmentState(&obs, seq, true, false, partial.Group)
			onPartial(withProbeReport(ctx, job, obs, protocol, started, partial.Result.failure))
		}
	}
	res := adapter.collect(ctx, job, sink)
	if ctx.Err() != nil && res.Status == "ok" {
		res = interruptedProbeResult(ctx)
	}
	obs := newObservationFromResult(job, res, started)
	if sink != nil {
		applyObservationSegmentState(&obs, seq+1, false, true, "final")
	}
	return withProbeReport(ctx, job, obs, protocol, started, res.failure)
}

func interruptedProbe(ctx context.Context, job entities.AgentlessJob, protocol string, started time.Time) entities.AgentlessObservation {
	res := interruptedProbeResult(ctx)
	return withProbeReport(ctx, job, newObservationFromResult(job, res, started), protocol, started, res.failure)
}

func interruptedProbeResult(ctx context.Context) Result {
	code := "probe_deadline"
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		code = "probe_cancelled"
	case errors.Is(context.Cause(ctx), errProbeBudget):
		code = "probe_budget_exhausted"
	}
	return Result{Status: "fail", Code: code, failure: ctx.Err()}
}

func withProbeReport(ctx context.Context, job entities.AgentlessJob, obs entities.AgentlessObservation, protocol string, started time.Time, failure error) entities.AgentlessObservation {
	if protocol == "snmp" && obs.CommandID != "" && obs.IsFinal &&
		(job.Config["snmp_basic_probe"] == true || job.Config["entity_sensor_probe"] == true) {
		obs.Payload["diagnostic_probe"] = true
	} else {
		obs = withMonitoringSamples(job, obs)
	}
	scope := monitoring.Scope{ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), CheckID: int64(job.CheckID)}
	outcome, reason := probeOutcome(ctx, protocol, obs, failure)
	report := monitoring.ProbeReport{Contract: monitoring.ProbeReportContract, Scope: scope, Protocol: protocol,
		Outcome: outcome, Reason: reason, ObservedAtMS: obs.ObservedAt.UnixMilli(), ElapsedMS: obs.ObservedAt.Sub(started).Milliseconds()}
	if report.Validate(scope, protocol) == nil {
		obs.ProbeReport = &report
	}
	return obs
}

func probeOutcome(ctx context.Context, protocol string, obs entities.AgentlessObservation, failure error) (string, string) {
	var networkError net.Error
	switch {
	case obs.Code == "unknown_type":
		return "unsupported", "protocol"
	case obs.IsPartial:
		return "partial", "segment"
	case obs.Status == "ok":
		return "ok", "none"
	case errors.Is(ctx.Err(), context.Canceled) || errors.Is(failure, context.Canceled):
		return "cancelled", "cancelled"
	case errors.Is(context.Cause(ctx), errProbeBudget):
		return "timeout", "budget"
	case errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(failure, context.DeadlineExceeded),
		errors.As(failure, &networkError) && networkError.Timeout():
		return "timeout", "deadline"
	}
	switch obs.Code {
	case "invalid_probe_policy", "missing_endpoint", "snmp_credential_unavailable":
		return "failed", "configuration"
	}
	if class, known := probeAdapters[protocol].failures[obs.Code]; known {
		return class.outcome, class.reason
	}
	return "failed", "execution"
}
