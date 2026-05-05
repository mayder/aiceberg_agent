package usecase

import (
	"context"
	"strconv"
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
	cfg      config.Config
	log      logger.Logger
	client   *remote.AgentlessHubClient
	outbox   ports.AgentlessOutboxRepo
	settings func() AgentlessSettings
	store    AgentlessTargetsStore
	mu       sync.RWMutex
	targets  []entities.AgentlessJob
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
	uc := &AgentlessHub{cfg: cfg, log: log, client: client, outbox: outbox, settings: settings, store: store}
	if store != nil {
		if jobs, err := store.Load(); err == nil && len(jobs) > 0 {
			uc.targets = jobs
		}
	}
	return uc
}

func (uc *AgentlessHub) PollAndRun(ctx context.Context) error {
	st := uc.getSettings()
	if !st.Enabled {
		return nil
	}
	jobs := uc.targetsSnapshot()
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		if uc.cfg.AgentlessDebug {
			uc.log.Info(formatAgentlessJob("agentless job start", job))
		}
		obsCount := 0
		appendObs := func(obs entities.AgentlessObservation) {
			if err := uc.outbox.Append(obs); err != nil {
				uc.log.Error(logger.KV("agentless outbox append failed",
					"job_id", job.CheckID,
					"job_type", job.Tipo,
					"err", err,
				))
				return
			}
			obsCount++
			if uc.cfg.AgentlessDebug {
				uc.log.Info(formatAgentlessObs("agentless job result", job, obs))
			}
		}
		obs := agentless.RunJobWithPartials(ctx, job, appendObs)
		appendObs(obs)
		if obsCount == 0 {
			uc.log.Error(logger.KV("agentless outbox append failed",
				"job_id", job.CheckID,
				"job_type", job.Tipo,
				"err", "nenhuma observacao enfileirada",
			))
		}
	}
	metrics.AddAgentlessJobs(len(jobs))
	uc.log.Info(logger.KV("agentless jobs executed",
		"batch_size", len(jobs),
	))
	return nil
}

func (uc *AgentlessHub) Flush(ctx context.Context) error {
	st := uc.getSettings()
	batchSize := st.FlushBatch
	if batchSize <= 0 {
		batchSize = uc.cfg.AgentlessFlushBatch
		if batchSize <= 0 {
			batchSize = 50
		}
	}
	batch, err := uc.outbox.ReadBatch(batchSize)
	if err != nil || len(batch) == 0 {
		return err
	}
	if err := uc.client.SendObservations(ctx, batch); err != nil {
		uc.log.Error(logger.KV("agentless send failed",
			"batch_size", len(batch),
			"err", err,
		))
		return err
	}
	ids := make([]string, 0, len(batch))
	for _, o := range batch {
		ids = append(ids, o.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error(logger.KV("agentless outbox ack failed",
			"batch_size", len(ids),
			"err", err,
		))
		return err
	}
	if uc.cfg.AgentlessDebug {
		uc.log.Info(logger.KV("agentless flushed batch",
			"batch_size", len(ids),
		))
	}
	uc.log.Info(logger.KV("agentless flushed ack",
		"batch_size", len(ids),
	))
	return nil
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
	if err := uc.syncTargets(ctx, opts); err != nil {
		return err
	}
	if err := uc.PollAndRun(ctx); err != nil {
		return err
	}
	return uc.Flush(ctx)
}

func (uc *AgentlessHub) SyncTargets(ctx context.Context) error {
	return uc.syncTargets(ctx, remote.AgentlessFetchOptions{})
}

func (uc *AgentlessHub) syncTargets(ctx context.Context, opts remote.AgentlessFetchOptions) error {
	st := uc.getSettings()
	if !st.Enabled {
		return nil
	}
	limit := st.JobsLimit
	if limit <= 0 {
		limit = uc.cfg.AgentlessJobsLimit
		if limit <= 0 {
			limit = 50
		}
	}
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
		return err
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
	uc.setTargets(jobs)
	if uc.store != nil {
		if err := uc.store.Save(jobs); err != nil {
			uc.log.Error(logger.KV("agentless targets save failed",
				"err", err,
			))
		}
	}
	return nil
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
	endpoint := ""
	if job.Endpoint != nil {
		endpoint = job.Endpoint.Tipo + ":" + job.Endpoint.Endereco
		if job.Endpoint.Porta != nil && *job.Endpoint.Porta > 0 {
			endpoint += ":" + strconv.Itoa(*job.Endpoint.Porta)
		}
		if job.Endpoint.TLSSNI != "" {
			endpoint += " sni=" + job.Endpoint.TLSSNI
		}
	}
	return logger.KV(prefix,
		"job_id", job.CheckID,
		"job_type", job.Tipo,
		"job", job.Tipo,
		"endpoint", endpoint,
	)
}

func formatAgentlessObs(prefix string, job entities.AgentlessJob, obs entities.AgentlessObservation) string {
	msg := obs.Message
	if len(msg) > 120 {
		msg = msg[:120]
	}
	return logger.KV(prefix,
		"job_id", job.CheckID,
		"job_type", job.Tipo,
		"job", job.Tipo,
		"status", obs.Status,
		"code", obs.Code,
		"latency_ms", obs.LatencyMs,
		"msg", msg,
	)
}
