package app

import (
	"context"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/relayprobe"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/agentless"
)

func startRelaySNMPAssignment(ctx context.Context, cfg config.Config, log logger.Logger) func() {
	settings, enabled := cfg.RelaySNMPAssignmentSettings()
	assignmentURL := cfg.HubURL
	if cfg.RelaySNMPHubReturnEnabled() {
		assignmentURL = cfg.APIEndpoint(relayprobe.AssignmentJobPath)
	}
	if !enabled || cfg.AgentlessOutboxPath == "" || !httpx.VerifiedHTTPS(assignmentURL, cfg.TLSInsecureSkip) {
		return func() {}
	}
	store, err := agentlessstore.NewBoltStore(cfg.AgentlessOutboxPath+".relay-assignment", cfg.AgentlessOutboxMaxMB)
	if err != nil {
		log.Error(logger.KV("delegated SNMP unavailable", "code", "relay_assignment_outbox_unavailable"))
		return func() {}
	}
	client := remote.NewRelaySNMPProbeClientWithIdentity(cfg, relayProbeIdentity(ctx, cfg))
	scope := monitoring.CollectorAssignmentScope{ClientID: int64(cfg.AgentClientID), HubID: settings.HubID, OwnerID: int64(cfg.AgentID)}
	worker := usecase.NewRelaySNMPAssignmentWorker(client, repositories.NewAgentlessOutboxRepository(store), store,
		scope, settings.PublicKey, func(ctx context.Context, job entities.AgentlessJob) entities.AgentlessObservation {
			return agentless.RunJobWithCredentials(ctx, job, nil, client.ResolveAssignmentCredentials)
		})
	runCtx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				if err := worker.Tick(runCtx); err != nil && runCtx.Err() == nil {
					log.Error(logger.KV("delegated SNMP pending", "code", "relay_assignment_cycle_failed"))
				}
			}
		}
	}()
	return func() { cancel(); wg.Wait(); _ = store.Close() }
}
