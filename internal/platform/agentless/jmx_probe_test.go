package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestJMXProbeReadsTheLocalJDKThroughVerifiedWorker(t *testing.T) {
	address := os.Getenv("AICEBERG_JMX_PROBE_FIXTURE_ADDRESS")
	if address == "" {
		t.Skip("run scripts/check-jmx-collector.py to start the JDK fixture")
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	const ref = "jmx_probe_profile:22"
	job := entities.AgentlessJob{
		Tipo: "jmx", ClienteID: 3, AtivoID: 52, CheckID: 72, BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{Tipo: "ip", ID: 17, Endereco: host, Porta: &port},
		Config: entities.AgentlessConfig{"credential_ref": ref, "metrics": []any{
			map[string]any{"id": "heap", "preset": "heap_used_bytes"},
			map[string]any{"id": "uptime", "preset": "uptime_ms"},
			map[string]any{"id": "threads", "preset": "live_threads"},
		}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract,
			BudgetMS: 6000, FreshnessSec: 180, CredentialRef: ref},
	}
	observation := RunJob(context.Background(), job)
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "jmx" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 3 {
		t.Fatalf("native JMX did not collect from the real JVM: %s", observation.Code)
	}
	for i, unit := range []string{"bytes", "milliseconds", "none"} {
		sample := observation.MonitoringSamples.Samples[i]
		if sample.Value == nil || *sample.Value <= 0 || sample.Unit != unit {
			t.Fatalf("unexpected JMX sample: %+v", sample)
		}
	}
	checkJMXRestrictions(t, job, observation)
}

func checkJMXRestrictions(t *testing.T, job entities.AgentlessJob, observation entities.AgentlessObservation) {
	t.Helper()
	settings, ok := config.AgentlessJMXSettings()
	selection, err := monitoring.JMXSelectors(job.Config)
	if !ok || err != nil {
		t.Fatal("integration fixture settings invalid")
	}
	profile, ok := loadJMXProfile(settings.Profiles, job, selection)
	if !ok {
		t.Fatal("fixture profile did not resolve")
	}
	wire, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{profile.User, profile.Password, "BEGIN CERTIFICATE", profile.Host, settings.JAR} {
		if bytes.Contains(wire, []byte(forbidden)) {
			t.Fatal("private fixture details leaked in observation")
		}
	}

	foreign := job
	foreign.ClienteID++
	if result := RunJob(context.Background(), foreign); result.Code != "jmx_credential_unavailable" {
		t.Fatalf("cross-tenant profile accepted: %s", result.Code)
	}
	t.Setenv("AICEBERG_JMX_WORKER_SHA256", strings.Repeat("0", 64))
	if result := RunJob(context.Background(), job); result.Code != "jmx_worker_unavailable" || len(result.Payload) != 0 {
		t.Fatalf("unverified JAR accepted: %s", result.Code)
	}
}

func TestJMXProbeRejectsUnexpectedOrNonIntegralWorkerData(t *testing.T) {
	for _, raw := range []string{
		`{"status":"ok","metrics":{"heap_used_bytes":-1}}`,
		`{"status":"ok","metrics":{"heap_used_bytes":1.2}}`,
		`{"status":"ok","metrics":{"unreviewed_preset":1}}`,
		`{"status":"ok","metrics":{"heap_used_bytes":0},"raw":"hidden"}`,
		`{"status":"ok","metrics":{"heap_used_bytes":9007199254740992}}`,
	} {
		if _, err := decodeJMXWorkerReply([]byte(raw), nil); err == nil {
			t.Fatal("unapproved JMX result accepted")
		}
	}
	reply, err := decodeJMXWorkerReply([]byte(`{"status":"ok","metrics":{"heap_used_bytes":0}}`), nil)
	if err != nil || len(reply.Metrics) != 1 || reply.Metrics["heap_used_bytes"] != 0 {
		t.Fatal("zero must remain a valid measurement")
	}
}
