package app

import (
	"context"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func startRelaySNMPProbe(ctx context.Context, cfg config.Config, log logger.Logger) (func(context.Context, entities.SelfHealCommand) (map[string]any, error), func()) {
	if cfg.Mode() != "relay" || cfg.AgentlessOutboxPath == "" {
		return nil, func() {}
	}
	// A dedicated durable file cannot mix with previous HUB routine observations.
	store, err := agentlessstore.NewBoltStore(cfg.AgentlessOutboxPath+".relay-probe", cfg.AgentlessOutboxMaxMB)
	if err != nil {
		log.Error(logger.KV("relay probe unavailable", "code", "relay_probe_outbox_unavailable"))
		return nil, func() {}
	}
	worker := usecase.NewRelaySNMPProbe("relay", remote.NewRelaySNMPProbeClientWithIdentity(cfg, relayProbeIdentity(ctx, cfg)), repositories.NewAgentlessOutboxRepository(store), agentless.RunJob)
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer := time.NewTicker(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-timer.C:
				if err := worker.Flush(runCtx); err != nil {
					log.Error(logger.KV("relay probe delivery pending", "code", "relay_probe_delivery_failed"))
				}
			}
		}
	}()
	return worker.Execute, func() { cancel(); wg.Wait(); _ = store.Close() }
}
