package agentless

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/crypto/ssh"
)

func TestSSHProbeAgainstRestrictedOpenSSH(t *testing.T) {
	path := os.Getenv("AICEBERG_SSH_OPENSSH_FIXTURE_PROFILE")
	if path == "" {
		t.Skip("set AICEBERG_SSH_OPENSSH_FIXTURE_PROFILE for a restricted loopback OpenSSH server")
	}
	requireSSHProfilePlatform(t)
	port, err := strconv.Atoi(os.Getenv("AICEBERG_SSH_OPENSSH_FIXTURE_PORT"))
	if err != nil || port < 1024 || port > 65535 {
		t.Fatal("fixture must use a high loopback port")
	}
	job := openSSHFixtureJob(port)
	selection, err := monitoring.SSHSelectors(job.Config)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := sshProbeEndpoint(job.Endpoint)
	profile, ok := loadSSHProfile(path, job, selection, target)
	if !ok {
		t.Fatal("fixture profile does not match the collector scope")
	}
	t.Setenv("AICEBERG_SSH_PROBE_PROFILES_FILE", path)
	observation := RunJob(context.Background(), job)
	assertOpenSSHObservation(t, observation, profile)
	verifyOpenSSHRejectsUnauthorizedKey(t, job, profile)
	t.Setenv("AICEBERG_SSH_PROBE_PROFILES_FILE", path)

	foreign := job
	foreign.AtivoID++
	if got := RunJob(context.Background(), foreign); got.Code != "ssh_credential_unavailable" {
		t.Fatalf("profile accepted an out-of-scope asset: %s", got.Code)
	}
	wrongPin := profile
	_, wrongKey := sshFixtureKey(t)
	wrongPin.HostKey = string(ssh.MarshalAuthorizedKey(wrongKey.PublicKey()))
	writeSSHFixtureProfile(t, wrongPin)
	got := RunJob(context.Background(), job)
	if got.Code != "ssh_host_key_mismatch" || got.Status != "fail" {
		t.Fatalf("wrong host pin was accepted: %s", got.Code)
	}
	if got.MonitoringSamples != nil {
		for _, sample := range got.MonitoringSamples.Samples {
			if sample.Value != nil {
				t.Fatal("host key rejection produced a measurement")
			}
		}
	}
	t.Setenv("AICEBERG_SSH_PROBE_PROFILES_FILE", path)
	assertOpenSSHObservation(t, RunJob(context.Background(), job), profile)
}

func verifyOpenSSHRejectsUnauthorizedKey(t *testing.T, job entities.AgentlessJob, profile sshProbeProfile) {
	t.Helper()
	profile.PrivateKey, _ = sshFixtureKey(t)
	profile.Passphrase = ""
	writeSSHFixtureProfile(t, profile)
	got := RunJob(context.Background(), job)
	if got.Status != "fail" || got.Code != "ssh_access_denied" || got.ProbeReport == nil ||
		got.ProbeReport.Reason != "configuration" || len(got.Payload) != 0 {
		t.Fatalf("OpenSSH authorization rejection misclassified: %s %+v", got.Code, got.ProbeReport)
	}
	wire, err := json.Marshal(got)
	if err != nil || bytes.Contains(wire, []byte(profile.Username)) || bytes.Contains(wire, []byte("PRIVATE KEY")) {
		t.Fatal("SSH rejection exposed credential material")
	}
}

func openSSHFixtureJob(port int) entities.AgentlessJob {
	const ref = "ssh_probe_profile:51"
	return entities.AgentlessJob{
		Tipo: "ssh", ClienteID: 3, AtivoID: 52, CheckID: 72, BaseIntervalSec: 60,
		Endpoint: &entities.AgentlessEndpoint{ID: 17, Tipo: "ip", Endereco: "127.0.0.1", Porta: &port},
		Config: entities.AgentlessConfig{"credential_ref": ref, "metrics": []any{
			map[string]any{"id": "load", "preset": "load_1"},
			map[string]any{"id": "uptime", "preset": "uptime_seconds"},
			map[string]any{"id": "mem", "preset": "memory_available_bytes"},
		}},
		ProbePolicy: &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract,
			BudgetMS: 3000, FreshnessSec: 180, CredentialRef: ref},
	}
}

func assertOpenSSHObservation(t *testing.T, observation entities.AgentlessObservation, profile sshProbeProfile) {
	t.Helper()
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Protocol != "ssh" ||
		observation.MonitoringSamples == nil || len(observation.MonitoringSamples.Samples) != 3 {
		t.Fatalf("OpenSSH did not produce the expected measurements: %s", observation.Code)
	}
	for i, unit := range []string{"none", "seconds", "bytes"} {
		sample := observation.MonitoringSamples.Samples[i]
		if sample.Unit != unit || sample.Value == nil || *sample.Value < 0 || (i == 1 && *sample.Value == 0) {
			t.Fatalf("invalid measurement from /proc for %s", unit)
		}
	}
	wire, err := json.Marshal(observation)
	if err != nil || bytes.Contains(wire, []byte(profile.Username)) ||
		bytes.Contains(wire, []byte("PRIVATE KEY")) || bytes.Contains(wire, []byte("MemAvailable")) {
		t.Fatal("SSH observation leaked private fixture data or raw output")
	}
}
