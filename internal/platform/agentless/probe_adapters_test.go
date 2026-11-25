package agentless

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestUnsupportedProbeReceiptDoesNotEchoProtocolOrPayload(t *testing.T) {
	job := entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "private input", Config: map[string]any{"opaque": "private input"}}
	observation := RunJob(context.Background(), job)
	if observation.Code != "unknown_type" || observation.ProbeReport == nil || observation.ProbeReport.Outcome != "unsupported" {
		t.Fatal("unsupported job lost its execution state")
	}
	report := observation.ProbeReport
	wire, err := json.Marshal(report)
	if err != nil || strings.Contains(string(wire), "private input") {
		t.Fatal("probe report echoed untrusted input")
	}
	scope := monitoring.Scope{ClientID: 3, AssetID: 52, CheckID: 72}
	if _, err := monitoring.DecodeProbeReport(wire, scope, "unsupported"); err != nil {
		t.Fatal(err)
	}
}

func TestProbeFailureClassesFollowTheSelectedAdapter(t *testing.T) {
	for _, fixture := range []struct{ protocol, code, outcome, reason string }{
		{"jmx", "jmx_timeout", "timeout", "budget"},
		{"jmx", "jmx_unavailable", "failed", "connection"},
		{"postgresql", "postgresql_statement_timeout", "timeout", "deadline"},
		{"postgresql", "postgresql_access_denied", "failed", "configuration"},
		{"mysql", "mysql_access_denied", "failed", "configuration"},
		{"openmetrics", httpAccessDenied, "failed", "configuration"},
		{"jolokia", "http_fail", "failed", "connection"},
		{"browser", "browser_timeout", "timeout", "budget"},
		{"snmp", "snmp_refused", "failed", "connection"},
		{"snmp", "snmp_unsupported_version", "unsupported", "version"},
		{"telnet", "telnet_response_limit", "failed", "response"},
		{"tcp", "jmx_timeout", "failed", "execution"},
	} {
		t.Run(fixture.protocol+"/"+fixture.code, func(t *testing.T) {
			observation := entities.AgentlessObservation{Status: "fail", Code: fixture.code}
			outcome, reason := probeOutcome(context.Background(), fixture.protocol, observation, nil)
			if outcome != fixture.outcome || reason != fixture.reason {
				t.Fatalf("got %s/%s, want %s/%s", outcome, reason, fixture.outcome, fixture.reason)
			}
		})
	}
}

func TestProbeSkipsAdapterWhenParentAlreadyEnded(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	adapter := regularProbe(func(context.Context, entities.AgentlessJob) Result {
		t.Fatal("the adapter started with an ended context")
		return Result{}
	}, nil)
	job := canonicalTCPJob(443)
	for _, test := range []struct {
		ctx             context.Context
		outcome, reason string
	}{
		{cancelled, "cancelled", "cancelled"},
		{expired, "timeout", "deadline"},
	} {
		observation := runProbeAdapter(test.ctx, job, "tcp", adapter, time.Now(), nil)
		assertInterruptedProbe(t, observation, test.outcome, test.reason)
	}
}

func TestProbeDiscardsSuccessReturnedAfterBudget(t *testing.T) {
	ctx, cancel := context.WithTimeoutCause(context.Background(), 40*time.Millisecond, errProbeBudget)
	defer cancel()
	adapter := regularProbe(func(ctx context.Context, _ entities.AgentlessJob) Result {
		<-ctx.Done()
		return Result{Status: "ok", Payload: map[string]any{"late": true}}
	}, nil)
	observation := runProbeAdapter(ctx, canonicalTCPJob(443), "tcp", adapter, time.Now(), nil)
	assertInterruptedProbe(t, observation, "timeout", "budget")
	if observation.Payload["late"] != nil {
		t.Fatal("late data escaped the probe")
	}
}

func TestSegmentedProbePreservesOnlyPartsEmittedBeforeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	adapter := probeAdapter{
		flushPartials: func(entities.AgentlessJob) bool { return true },
		collect: func(_ context.Context, _ entities.AgentlessJob, emit probeSegmentSink) Result {
			emit(probeSegment{Group: "before", Result: Result{Status: "ok"}})
			cancel()
			emit(probeSegment{Group: "after", Result: Result{Status: "ok"}})
			return Result{Status: "ok", Payload: map[string]any{"late": true}}
		},
	}
	job := referenceSNMPJob(161)
	var parts []entities.AgentlessObservation
	observation := runProbeAdapter(ctx, job, "snmp", adapter, time.Now(), func(part entities.AgentlessObservation) {
		parts = append(parts, part)
	})
	assertInterruptedProbe(t, observation, "cancelled", "cancelled")
	if len(parts) != 1 || parts[0].Payload["segment_group"] != "before" || !parts[0].IsPartial ||
		!observation.IsFinal || observation.SegmentSeq != 2 || observation.Payload["late"] != nil {
		t.Fatal("the final receipt or emitted partials did not respect cancellation")
	}
}

func assertInterruptedProbe(t *testing.T, observation entities.AgentlessObservation, outcome, reason string) {
	t.Helper()
	report := observation.ProbeReport
	if observation.Status != "fail" || report == nil || report.Outcome != outcome || report.Reason != reason {
		t.Fatalf("unexpected receipt after interruption: status=%s report=%+v", observation.Status, report)
	}
}

func TestTLSProbeHonorsParentCancellationDuringHandshake(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job := canonicalTCPJob(listener.Addr().(*net.TCPAddr).Port)
	job.Tipo = "tls"
	done := make(chan entities.AgentlessObservation, 1)
	go func() { done <- RunJob(ctx, job) }()
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(2 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	select {
	case observation := <-done:
		report := observation.ProbeReport
		if observation.Status != "fail" || report == nil || report.Outcome != "cancelled" {
			t.Fatal("cancelled handshake was not reported as cancelled", report)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("TLS handshake ignored cancellation")
	}
}
