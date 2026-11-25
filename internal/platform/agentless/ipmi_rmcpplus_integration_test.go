package agentless

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIPMIProbeAgainstLoopbackBMC(t *testing.T) {
	binary := os.Getenv("AICEBERG_IPMI_LOOPBACK_FIXTURE_TOOL")
	if binary == "" {
		t.Skip("set AICEBERG_IPMI_LOOPBACK_FIXTURE_TOOL for the loopback RMCP+ fixture")
	}
	port, err := strconv.Atoi(os.Getenv("AICEBERG_IPMI_LOOPBACK_FIXTURE_PORT"))
	if !filepath.IsAbs(binary) || !validIPMIBinary(binary) || err != nil || port < 1024 || port > 65535 {
		t.Fatal("expected an absolute ipmitool path and an unprivileged fixture port")
	}

	job, profile := ipmiFixture(t, "exit 99\n")
	job.Endpoint.Porta = &port
	job.ProbePolicy.BudgetMS = 5000
	t.Setenv("AICEBERG_IPMITOOL_PATH", binary)
	valid, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	configured := strings.Replace(string(valid), `"port":623`, `"port":`+strconv.Itoa(port), 1)
	if err := os.WriteFile(profile, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}

	assertReadings := func() {
		t.Helper()
		obs := RunJob(context.Background(), job)
		if obs.Status != "ok" || obs.ProbeReport == nil || obs.ProbeReport.Protocol != "ipmi" || obs.MonitoringSamples == nil {
			t.Fatalf("RMCP+ probe failed: %s", obs.Code)
		}
		samples := obs.MonitoringSamples.Samples
		if len(samples) != 3 || samples[0].Unit != "celsius" || samples[0].Value == nil || *samples[0].Value != 31 ||
			samples[1].Unit != "rpm" || samples[1].Value == nil || *samples[1].Value != 0 ||
			samples[2].Value != nil || samples[2].Gap != "missing" {
			t.Fatal("RMCP+ readings did not preserve units, zero or missing values")
		}
		wire, err := json.Marshal(obs)
		if err != nil || strings.Contains(string(wire), "fixture-secret") || strings.Contains(string(wire), "CPU Temp") {
			t.Fatal("probe exposed credentials or raw sensor identities")
		}
	}

	assertReadings()
	invalid := strings.Replace(configured, "fixture-secret", "wrong-password", 1)
	if err := os.WriteFile(profile, []byte(invalid), 0o600); err != nil {
		t.Fatal(err)
	}
	failed := RunJob(context.Background(), job)
	if failed.Status != "fail" || failed.Code != "ipmi_read_failed" ||
		failed.MonitoringSamples == nil || len(failed.MonitoringSamples.Samples) != 3 {
		t.Fatalf("wrong credential did not return a failed observation: %s", failed.Code)
	}
	for _, sample := range failed.MonitoringSamples.Samples {
		if sample.Value != nil || sample.Gap != "observation_failed" {
			t.Fatal("wrong credential returned a usable reading")
		}
	}
	if err := os.WriteFile(profile, []byte(configured), 0o600); err != nil {
		t.Fatal(err)
	}
	assertReadings()
}
