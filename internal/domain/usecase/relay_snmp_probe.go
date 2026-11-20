package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type RelaySNMPClient interface {
	FetchProbe(context.Context, string) (entities.RelaySNMPJob, error)
	ReportProbe(context.Context, entities.AgentlessObservation) error
}

// RelaySNMPProbe never enables the routine worker or changes authoritative ownership.
type RelaySNMPProbe struct {
	mode                 string
	client               RelaySNMPClient
	repo                 ports.AgentlessOutboxRepo
	run                  func(context.Context, entities.AgentlessJob) entities.AgentlessObservation
	probeMu, flushMu     sync.Mutex
	now                  func() time.Time
	lastProbe, nextFlush time.Time
	failures             int
}

func NewRelaySNMPProbe(mode string, client RelaySNMPClient, repo ports.AgentlessOutboxRepo, run func(context.Context, entities.AgentlessJob) entities.AgentlessObservation) *RelaySNMPProbe {
	return &RelaySNMPProbe{mode: mode, client: client, repo: repo, run: run, now: time.Now}
}

func (uc *RelaySNMPProbe) Execute(ctx context.Context, command entities.SelfHealCommand) (map[string]any, error) {
	if uc.mode != "relay" || uc.client == nil || uc.repo == nil || uc.run == nil {
		return nil, errors.New("relay_probe_unavailable")
	}
	if !uc.probeMu.TryLock() {
		return nil, errors.New("relay_probe_busy")
	}
	defer uc.probeMu.Unlock()
	if !validRelayProbeCommand(command) {
		return nil, errors.New("relay_probe_command_invalid")
	}
	if !uc.lastProbe.IsZero() && uc.now().Sub(uc.lastProbe) < 30*time.Second {
		return nil, errors.New("relay_probe_rate_limited")
	}
	if pending, _ := uc.repo.Len(); pending > 0 {
		return nil, errors.New("relay_probe_delivery_pending")
	}
	uc.lastProbe = uc.now()
	assignment, err := uc.client.FetchProbe(ctx, command.CommandID)
	if err != nil {
		return nil, errors.New("relay_probe_fetch_failed")
	}
	if !validRelaySNMPAssignment(assignment, command, uc.now()) {
		return nil, errors.New("relay_probe_assignment_invalid")
	}
	ctx, cancel := context.WithDeadline(ctx, time.Unix(assignment.ExpiresAt, 0))
	defer cancel()
	observation := uc.run(ctx, *assignment.Job)
	observation.CommandID, observation.CorrelationID = command.CommandID, command.CommandID
	observation.CheckID, observation.Message = assignment.Job.CheckID, ""
	observation.ObservedAt = observation.ObservedAt.UTC()
	if err = uc.repo.Append(observation); err != nil {
		return nil, errors.New("relay_probe_buffer_failed")
	}
	return map[string]any{"buffered": true, "check_id": observation.CheckID, "executor_id": assignment.ExecutorID,
		"owner_hub_id": assignment.OwnerHubID, "execution_id": assignment.ExecutionID, "delivery_confirmed": false}, nil
}

func validRelayProbeCommand(command entities.SelfHealCommand) bool {
	if command.Code != "diagnose_snmp_get" || command.Mode != "manual" || strings.TrimSpace(command.CommandID) == "" || len(command.CommandID) > 120 || len(command.Payload) != 1 {
		return false
	}
	return positiveRelayInt(command.Payload["execution_id"]) > 0
}

func positiveRelayInt(value any) int {
	switch n := value.(type) {
	case int:
		if n > 0 {
			return n
		}
	case float64:
		if n > 0 && n < 2147483647 && float64(int(n)) == n {
			return int(n)
		}
	}
	return 0
}

func validRelaySNMPAssignment(a entities.RelaySNMPJob, command entities.SelfHealCommand, now time.Time) bool {
	j := a.Job
	if a.Status != "ready" || a.ExecutionID != positiveRelayInt(command.Payload["execution_id"]) || a.ExecutorID < 1 || a.OwnerHubID < 1 || a.ExpiresAt <= now.Unix() || a.ExpiresAt > now.Add(20*time.Second).Unix() || j == nil {
		return false
	}
	if j.CommandID != command.CommandID || j.CheckID < 1 || j.ClienteID < 1 || j.AtivoID < 1 || j.Tipo != "snmp" || j.Endpoint == nil || strings.TrimSpace(j.Endpoint.Endereco) == "" || j.SNMP == nil {
		return false
	}
	if j.SNMP.Port < 0 || j.SNMP.Port > 65535 {
		return false
	}
	oid, _ := j.Config["snmp_basic_oid"].(string)
	allowed := oid == "1.3.6.1.2.1.1.1.0" || oid == "1.3.6.1.2.1.1.2.0" || oid == "1.3.6.1.2.1.1.3.0"
	return allowed && j.Config["snmp_basic_probe"] == true && (j.SNMP.Version == "v1" || j.SNMP.Version == "v2c") && j.TimeoutMs == 1500 && j.Retries == 0 && j.SNMP.Retries == 0 && j.SNMP.TimeoutMs == 1500
}

// Flush retries only persisted results. It never repeats a GET or acknowledges without API acceptance.
func (uc *RelaySNMPProbe) Flush(ctx context.Context) error {
	if uc.mode != "relay" {
		return errors.New("relay_probe_unavailable")
	}
	if !uc.flushMu.TryLock() {
		return nil
	}
	defer uc.flushMu.Unlock()
	if uc.client == nil || uc.repo == nil || uc.now().Before(uc.nextFlush) {
		return nil
	}
	rows, err := uc.repo.ReadBatch(4)
	if err != nil {
		return errors.New("relay_probe_outbox_read_failed")
	}
	for _, row := range rows {
		if err = uc.client.ReportProbe(ctx, row); err != nil {
			uc.backoff()
			return errors.New("relay_probe_delivery_failed")
		}
		if err = uc.repo.Ack([]string{row.ID}); err != nil {
			return errors.New("relay_probe_ack_failed")
		}
		uc.failures = 0
		uc.nextFlush = time.Time{}
	}
	return nil
}

func (uc *RelaySNMPProbe) backoff() {
	if uc.failures < 5 {
		uc.failures++
	}
	shift := uc.failures - 1
	if shift > 4 {
		shift = 4
	}
	uc.nextFlush = uc.now().Add(time.Duration(15*(1<<shift)) * time.Second)
}
