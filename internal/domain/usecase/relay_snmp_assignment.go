package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type RelayAssignmentClient interface {
	FetchAssignmentOffer(context.Context) (entities.RelaySNMPOffer, error)
	ClaimAssignment(context.Context, entities.RelaySNMPOffer) (entities.RelaySNMPAssignment, error)
	ReportAssignment(context.Context, entities.AgentlessObservation) error
}

type RelayAssignmentReplayStore interface {
	ReserveDelegatedSNMP(digest string, expiresMS, nowMS int64) (bool, error)
	RetainFencedDelegatedSNMP(id string) error
}

type RelaySNMPAssignmentWorker struct {
	client RelayAssignmentClient
	repo   ports.AgentlessOutboxRepo
	replay RelayAssignmentReplayStore
	scope  monitoring.CollectorAssignmentScope
	key    ed25519.PublicKey
	run    func(context.Context, entities.AgentlessJob) entities.AgentlessObservation
	now    func() time.Time
	mu     sync.Mutex
}

func NewRelaySNMPAssignmentWorker(client RelayAssignmentClient, repo ports.AgentlessOutboxRepo, replay RelayAssignmentReplayStore,
	scope monitoring.CollectorAssignmentScope, key ed25519.PublicKey, run func(context.Context, entities.AgentlessJob) entities.AgentlessObservation,
) *RelaySNMPAssignmentWorker {
	return &RelaySNMPAssignmentWorker{client: client, repo: repo, replay: replay, scope: scope, key: key, run: run, now: time.Now}
}

func (w *RelaySNMPAssignmentWorker) Tick(ctx context.Context) error {
	if !w.mu.TryLock() {
		return nil
	}
	defer w.mu.Unlock()
	if pending, err := w.reportPending(ctx); pending || err != nil {
		return err
	}
	offer, err := w.client.FetchAssignmentOffer(ctx)
	if err != nil || offer.Status == "idle" {
		return err
	}
	if offer.Status != "offered" || offer.CheckID <= 0 || offer.Epoch == 0 {
		return errors.New("relay_assignment_offer_invalid")
	}
	assignment, err := w.client.ClaimAssignment(ctx, offer)
	if err != nil || assignment.Status == "idle" {
		return err
	}
	return w.execute(ctx, offer, assignment)
}

func (w *RelaySNMPAssignmentWorker) reportPending(ctx context.Context) (bool, error) {
	pending, err := w.repo.ReadBatch(1)
	if err != nil {
		return false, errors.New("relay_assignment_outbox_read_failed")
	}
	if len(pending) == 0 {
		return false, nil
	}
	result := pending[0]
	err = w.client.ReportAssignment(ctx, result)
	if errors.Is(err, entities.ErrRelayAssignmentFenced) {
		return true, w.replay.RetainFencedDelegatedSNMP(result.ID)
	}
	if err != nil {
		return true, err
	}
	return true, w.repo.Ack([]string{result.ID})
}

func (w *RelaySNMPAssignmentWorker) execute(ctx context.Context, offer entities.RelaySNMPOffer, assignment entities.RelaySNMPAssignment) error {
	scope := w.scope
	scope.CheckID, scope.Epoch = offer.CheckID, offer.Epoch
	now := w.now()
	if assignment.Status != "ready" || assignment.Grant.Verify(scope, assignment.Job, w.key, now) != nil {
		return monitoring.ErrCollectorAssignmentInvalid
	}
	job, err := decodeAssignedSNMPJob(assignment.Job, scope)
	if err != nil {
		return err
	}
	cycle := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("relay-snmp-assignment.v1:%d:%d:%d:%s",
		scope.ClientID, scope.CheckID, scope.Epoch, assignment.Grant.JobSHA256))))
	executionCtx, cancel := context.WithDeadline(ctx, time.UnixMilli(assignment.Grant.ExpiresAtMS))
	defer cancel()
	if executionCtx.Err() != nil {
		return executionCtx.Err()
	}
	reserved, err := w.replay.ReserveDelegatedSNMP(cycle, assignment.Grant.ExpiresAtMS, now.UnixMilli())
	if err != nil || !reserved {
		return err
	}
	obs := w.run(executionCtx, job)
	if executionCtx.Err() != nil || !w.now().Before(time.UnixMilli(assignment.Grant.ExpiresAtMS)) {
		return errors.New("relay_assignment_interrupted")
	}
	obs.ID, obs.DedupeKey, obs.CheckID, obs.CommandID = cycle, cycle, job.CheckID, job.CommandID
	obs.IsPartial, obs.IsFinal = false, true
	obs.Assignment = &assignment.Grant
	if err := w.repo.Append(obs); err != nil {
		return err
	}
	// Use the durable copy for the first attempt and for retries.
	_, err = w.reportPending(ctx)
	return err
}

func decodeAssignedSNMPJob(raw []byte, scope monitoring.CollectorAssignmentScope) (entities.AgentlessJob, error) {
	var job entities.AgentlessJob
	if json.Unmarshal(raw, &job) != nil || int64(job.ClienteID) != scope.ClientID || int64(job.CheckID) != scope.CheckID ||
		job.AtivoID <= 0 || job.Tipo != "snmp" || strings.TrimSpace(job.CommandID) == "" || len(job.CommandID) > 100 ||
		job.Endpoint == nil || job.Endpoint.ID <= 0 || strings.TrimSpace(job.Endpoint.Endereco) == "" ||
		job.ProbePolicy == nil || job.SNMP == nil || job.SNMP.BindingID <= 0 || job.SNMP.ProfileID <= 0 ||
		*job.SNMP != job.SNMP.WithoutCredential() || job.CredentialDelivery != entities.AgentlessCredentialReferenceV1 {
		return entities.AgentlessJob{}, errors.New("relay_assignment_job_invalid")
	}
	return job, nil
}
