package agentless

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestRelayPilotSNMPv1AgainstNetSNMP(t *testing.T) {
	daemon := os.Getenv("AICEBERG_SNMPD_BIN")
	output := os.Getenv("AICEBERG_RELAY_PILOT_WIRE_FIXTURE")
	if daemon == "" || output == "" {
		t.Skip("requires the disposable Runtime relay pilot fixture")
	}
	parent := filepath.Dir(output)
	if filepath.Base(output) != "relay-pilot-wire.json" || filepath.Dir(parent) != "/tmp" ||
		!strings.HasPrefix(filepath.Base(parent), "aiceberg-assignment-test.") {
		t.Fatal("relay pilot output must be in the disposable Runtime fixture")
	}
	port := startNetSNMPPollingFixture(t, daemon)
	job := referenceSNMPJob(port)
	job.ClienteID, job.AtivoID, job.CheckID, job.Endpoint.ID = 7, 90, 43, 501
	job.SNMP.Version = "v1"
	delete(job.Config, "snmp_basic_probe")
	delete(job.Config, "snmp_basic_oid")
	job.Config["groups"] = []string{"system"}
	var captures []entities.AgentlessObservation
	for range 3 {
		result := RunJobWithCredentials(context.Background(), job, nil,
			func(_ context.Context, scoped entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
				return netSNMPPollingCredential(*scoped.SNMP, true), nil
			})
		applied, _ := result.Payload["runtime_applied"].(map[string]any)
		system, _ := result.Payload["sys"].(map[string]any)
		if result.Status != "ok" || result.ProbeReport == nil || result.ProbeReport.Outcome != "ok" ||
			applied["snmp_version"] != "v1" || system["."+pollingNameOID] != pollingSysName {
			t.Fatalf("recurring SNMPv1 poll missed the Net-SNMP fixture: status=%q code=%q version=%v system_values=%d",
				result.Status, result.Code, applied["snmp_version"], len(system))
		}
		captures = append(captures, result)
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(file).Encode(captures); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
