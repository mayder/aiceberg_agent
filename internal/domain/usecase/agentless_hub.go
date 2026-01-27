package usecase

import (
	"context"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

type AgentlessHub struct {
	cfg      config.Config
	log      logger.Logger
	client   *remote.AgentlessHubClient
	outbox   ports.AgentlessOutboxRepo
	settings func() AgentlessSettings
}

type AgentlessSettings struct {
	Enabled    bool
	PollSec    int
	FlushSec   int
	JobsLimit  int
	LockSec    int
	FlushBatch int
}

func NewAgentlessHub(cfg config.Config, log logger.Logger, client *remote.AgentlessHubClient, outbox ports.AgentlessOutboxRepo, settings func() AgentlessSettings) *AgentlessHub {
	return &AgentlessHub{cfg: cfg, log: log, client: client, outbox: outbox, settings: settings}
}

func (uc *AgentlessHub) PollAndRun(ctx context.Context) error {
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
	jobs, err := uc.client.FetchJobs(ctx, limit, true, lockSec)
	if err != nil {
		uc.log.Error("agentless jobs failed: " + err.Error())
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	for _, job := range jobs {
		obs := agentless.RunJob(ctx, job)
		if err := uc.outbox.Append(obs); err != nil {
			uc.log.Error("agentless outbox append failed: " + err.Error())
		}
	}
	uc.log.Info("agentless jobs executed n=" + strconv.Itoa(len(jobs)))
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
		uc.log.Error("agentless send failed: " + err.Error())
		return err
	}
	ids := make([]string, 0, len(batch))
	for _, o := range batch {
		ids = append(ids, o.ID)
	}
	if err := uc.outbox.Ack(ids); err != nil {
		uc.log.Error("agentless outbox ack failed: " + err.Error())
		return err
	}
	uc.log.Info("agentless flushed ack=" + strconv.Itoa(len(ids)))
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

func (uc *AgentlessHub) getSettings() AgentlessSettings {
	if uc.settings == nil {
		return AgentlessSettings{Enabled: true}
	}
	return uc.settings()
}
