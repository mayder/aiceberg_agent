package usecase

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

type AgentlessHub struct {
	cfg             config.Config
	log             logger.Logger
	client          *remote.AgentlessHubClient
	outbox          ports.AgentlessOutboxRepo
	settings        func() AgentlessSettings
	store           AgentlessTargetsStore
	mu              sync.RWMutex
	flushMu         sync.Mutex
	deliveryOrdinal uint64
	runSlot         chan struct{}
	probeAdmission  *agentlessProbeAdmission
	targets         []entities.AgentlessJob
}

type AgentlessSettings struct {
	Enabled    bool
	PollSec    int
	FlushSec   int
	JobsLimit  int
	LockSec    int
	FlushBatch int
}

type AgentlessCommandRequest struct {
	CommandID     string
	CorrelationID string
	CheckIDs      []int
	TimeoutMs     int
}

type AgentlessTargetsStore interface {
	Load() ([]entities.AgentlessJob, error)
	Save(jobs []entities.AgentlessJob) error
	Clear() error
}

func NewAgentlessHub(cfg config.Config, log logger.Logger, client *remote.AgentlessHubClient, outbox ports.AgentlessOutboxRepo, settings func() AgentlessSettings, store AgentlessTargetsStore) *AgentlessHub {
	uc := &AgentlessHub{cfg: cfg, log: log, client: client, outbox: outbox, settings: settings, store: store, runSlot: make(chan struct{}, 1),
		probeAdmission: newAgentlessProbeAdmission()}
	if store != nil {
		if jobs, err := store.Load(); err == nil && len(jobs) > 0 {
			// Previous versions persisted manual commands with routine targets.
			for _, job := range jobs {
				if job.CommandID == "" {
					uc.targets = append(uc.targets, job)
				}
			}
		}
	}
	return uc
}

func (uc *AgentlessHub) PollAndRun(ctx context.Context) error {
	if !uc.getSettings().Enabled {
		return nil
	}
	if err := uc.acquireRun(ctx); err != nil {
		return err
	}
	defer uc.releaseRun()
	headroom, empty, err := uc.agentlessHeadroom()
	if err != nil {
		return err
	}
	if !uc.getSettings().Enabled || headroom == 0 {
		return nil
	}
	return uc.runJobs(ctx, uc.targetsSnapshot(), agentlessBatchPolicy{limit: headroom, outboxEmpty: empty})
}

func (uc *AgentlessHub) runJobs(ctx context.Context, jobs []entities.AgentlessJob, policy agentlessBatchPolicy) error {
	if len(jobs) == 0 {
		return nil
	}
	var appendMu sync.Mutex
	policy.workers = agentlessBatchWorkers
	policy.admission = uc.probeAdmission
	stats, err := runAgentlessBatch(ctx, jobs, policy, func(jobCtx context.Context, job entities.AgentlessJob) (agentlessProbeOutcome, error) {
		return uc.runJob(jobCtx, job, &appendMu)
	})
	if err != nil {
		return err
	}
	metrics.AddAgentlessJobs(stats.dispatched)
	uc.log.Info(logger.KV("agentless jobs executed", "batch_size", stats.dispatched,
		"deferred", len(jobs)-stats.dispatched, "circuit_deferred", stats.circuitDeferred, "budget_deferred", stats.budgetDeferred,
		"observation_cost", stats.observationCost, "workers", agentlessBatchWorkers))
	return nil
}

func (uc *AgentlessHub) runJob(ctx context.Context, job entities.AgentlessJob, appendMu *sync.Mutex) (agentlessProbeOutcome, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if uc.cfg.AgentlessDebug {
		uc.log.Info(formatAgentlessJob("agentless job start", job))
	}
	var persistenceErr error
	appendObs := func(obs entities.AgentlessObservation) {
		if persistenceErr != nil {
			return
		}
		appendMu.Lock()
		err := uc.outbox.Append(obs)
		appendMu.Unlock()
		if err != nil {
			persistenceErr = err
			cancel()
			uc.log.Error(logger.KV("agentless outbox append failed", "job_id", job.CheckID, "job_type", job.Tipo, "err", err))
			return
		}
		if uc.cfg.AgentlessDebug {
			uc.log.Info(formatAgentlessObs("agentless job result", job, obs))
		}
	}
	obs := agentless.RunJobWithCredentials(ctx, job, appendObs, uc.client.ResolveSNMPCredentials)
	// An interrupted diagnostic still belongs to its command and can be
	// delivered later. A cancelled routine probe must not mark a check offline.
	diagnosticReceipt := obs.CommandID != "" && obs.IsFinal && !obs.IsPartial && obs.Payload["diagnostic_probe"] == true
	if ctx.Err() == nil || obs.Status == "ok" || diagnosticReceipt {
		appendObs(obs)
	}
	if ctx.Err() != nil {
		return agentlessProbeInterrupted, persistenceErr
	}
	return agentlessExecutionOutcome(obs), persistenceErr
}

func (uc *AgentlessHub) Prune(maxAge time.Duration) {
	// Optional: best-effort pruning if store supports it
	switch s := uc.outbox.(type) {
	case interface {
		Prune(time.Duration) (int, error)
	}:
		_, _ = s.Prune(maxAge)
	case interface{ Prune(time.Duration) int }:
		_ = s.Prune(maxAge)
	}
}

func (uc *AgentlessHub) CollectNow(ctx context.Context) {
	_ = uc.PollAndRun(ctx)
	_ = uc.Flush(ctx)
}

func (uc *AgentlessHub) CollectCommand(ctx context.Context, req AgentlessCommandRequest) error {
	opts := remote.AgentlessFetchOptions{
		CommandID:     strings.TrimSpace(req.CommandID),
		CorrelationID: strings.TrimSpace(req.CorrelationID),
		CheckIDs:      req.CheckIDs,
	}
	if opts.CommandID == "" || len(opts.CheckIDs) == 0 {
		return nil
	}
	if !uc.getSettings().Enabled {
		return errors.New("agentless collection disabled")
	}
	if err := uc.runCommand(ctx, opts); err != nil {
		return err
	}
	err := uc.Flush(ctx)
	if agentlessDeliveryRetryable(err) {
		return &AgentlessPendingDeliveryError{cause: err}
	}
	return err
}

// AgentlessPendingDeliveryError leaves a collected observation available for outbox retry.
type AgentlessPendingDeliveryError struct{ cause error }

func (e *AgentlessPendingDeliveryError) Error() string { return e.cause.Error() }
func (e *AgentlessPendingDeliveryError) Unwrap() error { return e.cause }

var ErrAgentlessOutboxBusy = errors.New("agentless outbox admission limit reached")

func agentlessDeliveryRetryable(err error) bool {
	var delivery *remote.AgentlessDeliveryError
	if errors.As(err, &delivery) {
		return !delivery.PayloadRejected && (delivery.HTTPStatus == 0 || delivery.HTTPStatus == 429 || delivery.HTTPStatus >= 500)
	}
	var network net.Error
	return errors.As(err, &network)
}

func (uc *AgentlessHub) runCommand(ctx context.Context, opts remote.AgentlessFetchOptions) error {
	if err := uc.acquireRun(ctx); err != nil {
		return err
	}
	defer uc.releaseRun()
	st := uc.getSettings()
	if !st.Enabled {
		return errors.New("agentless collection disabled")
	}
	headroom, _, err := uc.agentlessHeadroom()
	if err != nil {
		return err
	}
	if headroom == 0 {
		return ErrAgentlessOutboxBusy
	}
	jobs, err := uc.fetchJobs(ctx, st, opts)
	if err != nil {
		return err
	}
	jobs = commandJobs(jobs, opts)
	if len(jobs) == 0 {
		return errors.New("no eligible agentless job for command")
	}
	headroom, empty, err := uc.agentlessHeadroom()
	if err != nil {
		return err
	}
	cost := 0
	for _, job := range jobs {
		cost += agentless.ObservationCost(job)
	}
	if cost > headroom && (!empty || len(jobs) != 1) {
		return ErrAgentlessOutboxBusy
	}
	return uc.runJobs(ctx, jobs, agentlessBatchPolicy{limit: headroom, outboxEmpty: empty, manual: true})
}

func commandJobs(jobs []entities.AgentlessJob, opts remote.AgentlessFetchOptions) []entities.AgentlessJob {
	requested := make(map[int]bool, len(opts.CheckIDs))
	for _, id := range opts.CheckIDs {
		if id > 0 {
			requested[id] = true
		}
	}
	matched := jobs[:0]
	for _, job := range jobs {
		if !requested[job.CheckID] || job.CommandID != opts.CommandID ||
			(opts.CorrelationID != "" && job.CorrelationID != opts.CorrelationID) {
			continue
		}
		matched = append(matched, job)
		delete(requested, job.CheckID)
	}
	return matched
}

func (uc *AgentlessHub) acquireRun(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case uc.runSlot <- struct{}{}:
		if err := ctx.Err(); err != nil {
			uc.releaseRun()
			return err
		}
		return nil
	}
}

func (uc *AgentlessHub) releaseRun() { <-uc.runSlot }

func (uc *AgentlessHub) SyncTargets(ctx context.Context) error {
	st := uc.getSettings()
	if !st.Enabled {
		return nil
	}
	headroom, _, err := uc.agentlessHeadroom()
	if err != nil {
		return err
	}
	if headroom == 0 {
		uc.setTargets(nil)
		uc.log.Info(logger.KV("agentless collection backpressure", "outbox_headroom", headroom))
		return nil
	}
	st.JobsLimit = min(uc.agentlessJobsLimit(st), headroom)
	jobs, err := uc.fetchJobs(ctx, st, remote.AgentlessFetchOptions{})
	if err != nil {
		return err
	}
	uc.setTargets(jobs)
	if uc.store != nil {
		if err := uc.store.Save(jobs); err != nil {
			uc.log.Error(logger.KV("agentless targets save failed", "err", err))
		}
	}
	return nil
}

func (uc *AgentlessHub) agentlessHeadroom() (int, bool, error) {
	countLimit := agentlessFlushBatches * uc.flushBatchSize()
	var pending int
	byteSlots := int64(countLimit)
	if admission, ok := uc.outbox.(ports.AgentlessOutboxAdmission); ok {
		var err error
		pending, byteSlots, err = admission.AdmissionState()
		if err != nil {
			return 0, false, err
		}
	} else {
		pending, _ = uc.outbox.Len()
	}
	countSlots := max(0, countLimit-pending)
	return int(min(int64(countSlots), byteSlots)), pending == 0, nil
}

func (uc *AgentlessHub) agentlessJobsLimit(st AgentlessSettings) int {
	if st.JobsLimit > 0 {
		return st.JobsLimit
	}
	if uc.cfg.AgentlessJobsLimit > 0 {
		return uc.cfg.AgentlessJobsLimit
	}
	return 50
}

func (uc *AgentlessHub) fetchJobs(ctx context.Context, st AgentlessSettings, opts remote.AgentlessFetchOptions) ([]entities.AgentlessJob, error) {
	limit := uc.agentlessJobsLimit(st)
	lockSec := st.LockSec
	if lockSec <= 0 {
		lockSec = uc.cfg.AgentlessLockSec
		if lockSec <= 0 {
			lockSec = 60
		}
	}
	jobs, err := uc.client.FetchJobsWithOptions(ctx, limit, true, lockSec, opts)
	if err != nil {
		uc.log.Error(logger.KV("agentless jobs failed",
			"limit", limit,
			"lock_sec", lockSec,
			"command_id", opts.CommandID,
			"correlation_id", opts.CorrelationID,
			"err", err,
		))
		return nil, err
	}
	if uc.cfg.AgentlessDebug {
		uc.log.Info(logger.KV("agentless jobs fetched",
			"batch_size", len(jobs),
			"limit", limit,
			"lock_sec", lockSec,
			"command_id", opts.CommandID,
			"correlation_id", opts.CorrelationID,
		))
	}
	return jobs, nil
}

func (uc *AgentlessHub) getSettings() AgentlessSettings {
	if uc.settings == nil {
		return AgentlessSettings{Enabled: true}
	}
	return uc.settings()
}

func (uc *AgentlessHub) setTargets(jobs []entities.AgentlessJob) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.targets = jobs
}

func (uc *AgentlessHub) targetsSnapshot() []entities.AgentlessJob {
	uc.mu.RLock()
	defer uc.mu.RUnlock()
	if len(uc.targets) == 0 {
		return nil
	}
	cp := make([]entities.AgentlessJob, len(uc.targets))
	copy(cp, uc.targets)
	return cp
}

func formatAgentlessJob(prefix string, job entities.AgentlessJob) string {
	var endpointID int
	if job.Endpoint != nil {
		endpointID = job.Endpoint.ID
	}
	return logger.KV(prefix,
		"job_id", job.CheckID,
		"job_type", job.Tipo,
		"job", job.Tipo,
		"endpoint_id", endpointID,
	)
}

func formatAgentlessObs(prefix string, job entities.AgentlessJob, obs entities.AgentlessObservation) string {
	return logger.KV(prefix,
		"job_id", job.CheckID,
		"job_type", job.Tipo,
		"job", job.Tipo,
		"status", obs.Status,
		"code", obs.Code,
		"latency_ms", obs.LatencyMs,
	)
}
