package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	store "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type fakeRelayProbeClient struct {
	assignment       entities.RelaySNMPJob
	fetches, reports int
	reportErr        error
}

func (f *fakeRelayProbeClient) FetchProbe(context.Context, string) (entities.RelaySNMPJob, error) {
	f.fetches++
	return f.assignment, nil
}
func (f *fakeRelayProbeClient) ReportProbe(context.Context, entities.AgentlessObservation) error {
	f.reports++
	return f.reportErr
}

func relayProbeFixture(now time.Time) (entities.SelfHealCommand, entities.RelaySNMPJob) {
	command := entities.SelfHealCommand{CommandID: "fixture-command-0001", Code: "diagnose_snmp_get", Mode: "manual", Payload: map[string]any{"execution_id": 12}}
	job := &entities.AgentlessJob{CheckID: 72, AtivoID: 52, ClienteID: 3, Tipo: "snmp", TimeoutMs: 1500, CommandID: command.CommandID,
		Endpoint: &entities.AgentlessEndpoint{Endereco: "192.0.2.20"}, SNMP: &entities.AgentlessSnmpProfile{Version: "v1", Community: "fixture-snmp-secret", TimeoutMs: 1500},
		Config: entities.AgentlessConfig{"snmp_basic_probe": true, "snmp_basic_oid": "1.3.6.1.2.1.1.3.0"}}
	return command, entities.RelaySNMPJob{Status: "ready", ExecutionID: 12, ExecutorID: 424, OwnerHubID: 84, ExpiresAt: now.Add(15 * time.Second).Unix(), Job: job}
}

func TestRelayProbeDurableDeliverySurvivesRestartWithoutRepeatingGET(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	command, assignment := relayProbeFixture(now)
	client := &fakeRelayProbeClient{assignment: assignment, reportErr: errors.New("fixture transport unavailable")}
	path := filepath.Join(t.TempDir(), "relay-probe.db")
	disk, err := store.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	gets := 0
	run := func(context.Context, entities.AgentlessJob) entities.AgentlessObservation {
		gets++
		return entities.AgentlessObservation{ID: "fixture-result-1", Status: "ok", IsFinal: true, ObservedAt: now, Payload: map[string]any{"diagnostic_probe": true}}
	}
	worker := NewRelaySNMPProbe("relay", client, repositories.NewAgentlessOutboxRepository(disk), run)
	worker.now = func() time.Time { return now }
	evidence, err := worker.Execute(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if evidence["buffered"] != true || evidence["delivery_confirmed"] != false || gets != 1 {
		t.Fatalf("incorrect buffered state: %+v", evidence)
	}
	if err = worker.Flush(context.Background()); err == nil {
		t.Fatal("missing ACK accepted")
	}
	if pending, _ := worker.repo.Len(); pending != 1 {
		t.Fatal("failed delivery was removed")
	}
	if err = disk.Close(); err != nil {
		t.Fatal(err)
	}
	disk, err = store.NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	worker = NewRelaySNMPProbe("relay", client, repositories.NewAgentlessOutboxRepository(disk), run)
	worker.now = func() time.Time { return now }
	if _, err = worker.Execute(context.Background(), command); err == nil {
		t.Fatal("new GET allowed while delivery pending")
	}
	if gets != 1 || client.fetches != 1 {
		t.Fatal("restart repeated GET or fetch")
	}
	rows, err := worker.repo.ReadBatch(4)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(rows)
	if strings.Contains(string(data), "fixture-snmp-secret") {
		t.Fatal("credential persisted in diagnostic outbox")
	}
	client.reportErr = nil
	if err = worker.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pending, _ := worker.repo.Len(); pending != 0 {
		t.Fatal("accepted delivery remains pending")
	}
	if gets != 1 || client.reports != 2 {
		t.Fatal("delivery retry repeated collection")
	}
}

func TestRelayProbeRejectsExpiredOrUnexpectedAssignmentBeforeCollector(t *testing.T) {
	for _, kind := range []string{"expired", "version", "oid", "execution", "command", "role", "payload"} {
		t.Run(kind, func(t *testing.T) {
			now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
			command, a := relayProbeFixture(now)
			mode := "relay"
			switch kind {
			case "expired":
				a.ExpiresAt = now.Unix()
			case "version":
				a.Job.SNMP.Version = "v3"
			case "oid":
				a.Job.Config["snmp_basic_oid"] = "1.3.6.1.4.1.99999.0"
			case "execution":
				a.ExecutionID = 99
			case "command":
				a.Job.CommandID = "wrong"
			case "role":
				mode = "hub"
			case "payload":
				command.Payload["host"] = "192.0.2.99"
			}
			client := &fakeRelayProbeClient{assignment: a}
			repo := repositories.NewAgentlessOutboxRepository(store.NewMemStore())
			worker := NewRelaySNMPProbe(mode, client, repo, func(context.Context, entities.AgentlessJob) entities.AgentlessObservation {
				t.Fatal("invalid assignment executed")
				return entities.AgentlessObservation{}
			})
			worker.now = func() time.Time { return now }
			if _, err := worker.Execute(context.Background(), command); err == nil {
				t.Fatal("invalid assignment accepted")
			}
			if pending, _ := repo.Len(); pending != 0 {
				t.Fatal("invalid result stored")
			}
		})
	}
}

func TestRelayProbeRateLimitAndDeliveryBackoffNeverRetryCollection(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	command, a := relayProbeFixture(now)
	client := &fakeRelayProbeClient{assignment: a, reportErr: errors.New("fixture transport unavailable")}
	repo := repositories.NewAgentlessOutboxRepository(store.NewMemStore())
	worker := NewRelaySNMPProbe("relay", client, repo, func(context.Context, entities.AgentlessJob) entities.AgentlessObservation {
		return entities.AgentlessObservation{ID: "fixture-result-1", ObservedAt: now}
	})
	worker.now = func() time.Time { return now }
	if _, err := worker.Execute(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if _, err := worker.Execute(context.Background(), command); err == nil {
		t.Fatal("rate limit missing")
	}
	_ = worker.Flush(context.Background())
	_ = worker.Flush(context.Background())
	if client.reports != 1 || client.fetches != 1 {
		t.Fatal("backoff or GET rate limit missing")
	}
	now = now.Add(16 * time.Second)
	_ = worker.Flush(context.Background())
	if client.reports != 2 || client.fetches != 1 {
		t.Fatal("result retry repeated GET")
	}
}
