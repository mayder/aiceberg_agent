package agentless

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestSNMPProbeUsesReferencedCredentialOnlyDuringExecution(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	job := referenceSNMPJob(conn.LocalAddr().(*net.UDPAddr).Port)
	job.CommandID = "fixture-diagnostic"
	job.SNMP.Version = "v1"
	done := make(chan error, 1)
	go func() { done <- serveBasicFixture(conn, "", false) }()
	observation := RunJobWithCredentials(context.Background(), job, nil, func(ctx context.Context, scoped entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second || scoped.ProbePolicy.CredentialRef != "snmp_profile:41" ||
			scoped.CommandID != "fixture-diagnostic" {
			t.Error("resolver did not inherit the job budget or scope")
		}
		profile := *scoped.SNMP
		profile.Community = "fixture-basic-private"
		return &profile, nil
	})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if observation.Status != "ok" || observation.ProbeReport == nil || observation.ProbeReport.Outcome != "ok" || job.SNMP.Community != "" {
		t.Fatalf("reference probe did not preserve the result or cached job: %s", observation.Code)
	}
	if observation.CommandID != job.CommandID || observation.Payload["diagnostic_probe"] != true {
		t.Fatal("successful probe lost diagnostic scope")
	}
	wire, _ := json.Marshal(observation)
	if strings.Contains(string(wire), "fixture-basic-private") {
		t.Fatal("credential leaked into the probe observation")
	}
}

func TestSNMPProbeResolutionFailureNeverTouchesTarget(t *testing.T) {
	conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	job := referenceSNMPJob(conn.LocalAddr().(*net.UDPAddr).Port)
	job.CommandID = "fixture-diagnostic-failure"
	observation := RunJobWithCredentials(context.Background(), job, nil, func(context.Context, entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
		return nil, errors.New("sensitive resolver fixture")
	})
	if observation.Code != "snmp_credential_unavailable" || observation.ProbeReport == nil ||
		observation.ProbeReport.Reason != "configuration" || observation.Message != "" ||
		observation.CommandID != job.CommandID || observation.Payload["diagnostic_probe"] != true {
		t.Fatal("credential failure escaped the typed receipt")
	}
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	if _, _, err := conn.ReadFrom(make([]byte, 2048)); err == nil {
		t.Fatal("SNMP target received a packet before credentials were available")
	}
}

func TestSNMPProbeCancellationDuringCredentialLookupIsReported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job := referenceSNMPJob(161)
	job.CommandID = "fixture-diagnostic-cancelled"
	observation := RunJobWithCredentials(ctx, job, nil, func(lookupContext context.Context, _ entities.AgentlessJob) (*entities.AgentlessSnmpProfile, error) {
		cancel()
		<-lookupContext.Done()
		return nil, lookupContext.Err()
	})
	if observation.ProbeReport == nil || observation.ProbeReport.Outcome != "cancelled" ||
		observation.Code != "probe_cancelled" || observation.Payload["diagnostic_probe"] != true ||
		observation.MonitoringSamples != nil || !observation.IsFinal {
		t.Fatal("credential lookup cancellation was reported as a target failure")
	}
}

func referenceSNMPJob(port int) entities.AgentlessJob {
	job := newSNMPJob()
	job.AtivoID, job.ClienteID, job.IntervalSec = 52, 3, 60
	job.Endpoint.ID, job.Endpoint.Endereco = 17, "127.0.0.1"
	job.SNMP.Port, job.SNMP.Version, job.SNMP.Community = port, "v2c", ""
	job.SNMP.BindingID, job.SNMP.ProfileID = 61, 41
	job.CredentialDelivery = entities.AgentlessCredentialReferenceV1
	job.ProbePolicy = &monitoring.ProbePolicy{Contract: monitoring.ProbePolicyContract, BudgetMS: 2000, FreshnessSec: 180, CredentialRef: "snmp_profile:41"}
	job.Config["snmp_basic_probe"], job.Config["snmp_basic_oid"] = true, "1.3.6.1.2.1.1.3.0"
	return job
}
