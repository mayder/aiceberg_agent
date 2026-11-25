package agentless

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestIPMIProbeUsesScopedReadOnlyCommandAndPreservesUnits(t *testing.T) {
	job, _ := ipmiFixture(t, `test "$*" = "-c -I lanplus -C 17 -L USER -N 1 -R 1 -H 127.0.0.1 -p 623 -U fixture -E sensor list" || exit 1
test "$IPMI_PASSWORD" = "fixture-secret" || exit 1
printf '%s\n' 'CPU Temp,31.000,degrees C,ok,na,na,na,na,na,na' 'Fan 1,0,RPM,lnc,na,na,na,na,na,na' 'Missing,na,degrees C,na,na,na,na,na,na,na'
printf 'fixture-secret stderr' >&2
`)
	obs := RunJob(context.Background(), job)
	if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "ipmi" || obs.MonitoringSamples == nil {
		t.Fatalf("IPMI probe failed: %s", obs.Code)
	}
	samples := obs.MonitoringSamples.Samples
	if len(samples) != 3 || samples[0].Unit != "celsius" || samples[0].Value == nil || *samples[0].Value != 31 ||
		samples[1].Unit != "rpm" || samples[1].Value == nil || *samples[1].Value != 0 ||
		samples[2].Value != nil || samples[2].Gap != "missing" {
		t.Fatal("qualified values, units, zero or missing state changed")
	}
	wire, err := json.Marshal(obs)
	if err != nil || strings.Contains(string(wire), "fixture-secret") || strings.Contains(string(wire), "CPU Temp") {
		t.Fatal("probe exposed credentials or raw sensor identities")
	}
}

func TestIPMIProbeRejectsWrongScopeBeforeExecuting(t *testing.T) {
	job, fixture := ipmiFixture(t, "exit 21\n")
	for _, change := range []func(*entities.AgentlessJob){
		func(job *entities.AgentlessJob) { job.ClienteID++ },
		func(job *entities.AgentlessJob) { job.CheckID++ },
		func(job *entities.AgentlessJob) { job.AtivoID++ },
		func(job *entities.AgentlessJob) { job.Endpoint.ID++ },
		func(job *entities.AgentlessJob) { job.Endpoint.Endereco = "127.0.0.2" },
		func(job *entities.AgentlessJob) { port := 624; job.Endpoint.Porta = &port },
		func(job *entities.AgentlessJob) { job.ProbePolicy.CredentialRef = "ipmi_profile:9" },
	} {
		attempt := job
		endpoint, policy := *job.Endpoint, *job.ProbePolicy
		attempt.Endpoint, attempt.ProbePolicy = &endpoint, &policy
		change(&attempt)
		if obs := RunJob(context.Background(), attempt); obs.Code != "ipmi_credential_unavailable" {
			t.Fatalf("mismatched scope reached executable: %s", obs.Code)
		}
	}
	if err := os.Chmod(fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	if obs := RunJob(context.Background(), job); obs.Code != "ipmi_credential_unavailable" {
		t.Fatal("public credential file was accepted")
	}
}

func TestIPMIProbeHonorsDeadlineAndOutputLimit(t *testing.T) {
	job, _ := ipmiFixture(t, "exec /bin/sleep 4\n")
	job.ProbePolicy.BudgetMS = 100
	start := time.Now()
	obs := RunJob(context.Background(), job)
	if obs.ProbeReport == nil || obs.ProbeReport.Outcome != "timeout" || time.Since(start) > 2*time.Second {
		t.Fatalf("bounded command did not stop: %s", obs.Code)
	}
	job, _ = ipmiFixture(t, "exec /bin/dd if=/dev/zero bs=1024 count=300\n")
	if obs := RunJob(context.Background(), job); obs.Code != "invalid_ipmi_response" || len(obs.Payload) != 0 {
		t.Fatalf("oversized output not isolated: %s", obs.Code)
	}
}

func TestIPMIQualifiedSamplesIgnoreAmbiguousAndWrongUnitRows(t *testing.T) {
	selector, err := monitoring.IPMISelectors(ipmiTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	output := "CPU Temp,31,degrees C,ok,na,na,na,na,na,na\n" +
		"CPU Temp,32,degrees C,ok,na,na,na,na,na,na\n" +
		"Fan 1,1200,volts,ok,na,na,na,na,na,na\n" +
		"Missing,0,degrees C,ns,na,na,na,na,na,na\n"
	if values, err := parseIPMISensors([]byte(output), selector); err != nil || len(values) != 0 {
		t.Fatal("ambiguous, unavailable or incorrectly scaled readings were accepted")
	}
}

func ipmiTestConfig() entities.AgentlessConfig {
	return entities.AgentlessConfig{"credential_ref": "ipmi_profile:8", "metrics": []any{
		map[string]any{"id": "cpu_temp", "sensor": "CPU Temp", "unit": "celsius"},
		map[string]any{"id": "fan", "sensor": "Fan 1", "unit": "rpm"},
		map[string]any{"id": "absent", "sensor": "Missing", "unit": "celsius"},
	}}
}

func ipmiFixture(t *testing.T, body string) (entities.AgentlessJob, string) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("read-only local IPMI executable fixture requires Unix")
	}
	// Resolve the protected directory before writing private credential fixtures.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary, file := filepath.Join(dir, "ipmitool"), filepath.Join(dir, "profiles.json")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := `{"profiles":[{"reference":"ipmi_profile:8","client_id":3,"asset_id":52,"check_id":72,
		"endpoint_id":17,"ip":"127.0.0.1","port":623,"username":"fixture","password":"fixture-secret"}]}`
	if err := os.WriteFile(file, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AICEBERG_IPMITOOL_PATH", binary)
	t.Setenv("AICEBERG_IPMI_PROFILES_FILE", file)
	return entities.AgentlessJob{ClienteID: 3, AtivoID: 52, CheckID: 72, Tipo: "ipmi", BaseIntervalSec: 60,
		Config: ipmiTestConfig(), Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: "127.0.0.1"},
		// Leave room for process startup on loaded runners. The deadline test sets its own 100 ms budget.
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 3000, FreshnessSec: 180, CredentialRef: "ipmi_profile:8"}}, file
}
