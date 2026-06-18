package app

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/host"
	ps "github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/common/version"
	agentlessstore "github.com/you/aiceberg_agent/internal/data/local/agentless"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	agentlessremote "github.com/you/aiceberg_agent/internal/data/remote"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/channel"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	agentruntime "github.com/you/aiceberg_agent/internal/domain/runtime"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/health"
	"github.com/you/aiceberg_agent/internal/interfaces/hub"
	"github.com/you/aiceberg_agent/internal/platform/collectors/custommetrics"
	"github.com/you/aiceberg_agent/internal/platform/collectors/networkcapture"
	"github.com/you/aiceberg_agent/internal/platform/collectors/oslogs"
	"github.com/you/aiceberg_agent/internal/platform/collectors/sysmetrics"
	"github.com/you/aiceberg_agent/internal/platform/modechange"
)

func Run(ctx context.Context, cfg config.Config, log logger.Logger) error {
	startedAt := time.Now()
	// Adapters mínimos
	store := selectOutbox(cfg, log)
	outboxRepo := repositories.NewOutboxRepository(store)
	prefStore := prefs.NewStore(cfg.PrefsPath)
	_, _ = prefStore.Load()

	mode := cfg.Mode()

	pruneOpts := outbox.PruneOptions{MaxPerAgent: 10}
	if mode == "hub" {
		pruneOpts.MaxPerAgent = 100
		pruneOpts.MaxAge = 10 * time.Minute
	}
	if cfg.OutboxMaxPerAgent > 0 {
		pruneOpts.MaxPerAgent = cfg.OutboxMaxPerAgent
	}
	pruneStore := func(store repositories.Store, label string) {
		pruner, ok := store.(interface {
			Prune(outbox.PruneOptions) (int, error)
		})
		if !ok {
			return
		}
		opts := pruneOpts
		opts.Now = time.Now()
		removed, err := pruner.Prune(opts)
		if err != nil {
			log.Error(logger.KV("outbox prune failed",
				"target", label,
				"err", err,
			))
			return
		}
		if removed > 0 {
			log.Info(logger.KV("outbox pruned",
				"target", label,
				"removed", removed,
			))
		}
	}
	pruneStore(store, "main")

	if !cfg.SkipBootstrap {
		if err := bootstrap(ctx, cfg, log); err != nil {
			log.Fatal("bootstrap failed", "op", "bootstrap", "err", err)
		}
	}

	// Use cases
	authHeader := ""
	if cfg.Agent.Token != "" {
		authHeader = "Token " + cfg.Agent.Token
	} else if cfg.APIKey != "" {
		authHeader = "Bearer " + cfg.APIKey
	}
	identityHeader := cfg.AgentIdentityHeader("")

	var tx ports.Transport
	if mode == "relay" {
		tx = transport.NewHubClient(cfg)
	} else {
		tx = transport.NewHTTPJSONClient(cfg)
	}

	var counters obsCounters
	proc := processHandle()

	collector := sysmetrics.New(outboxRepo.Len, prefStore.Get, func() sysmetrics.AgentRuntimeStats {
		return sysmetrics.AgentRuntimeStats{
			FlushOK:        counters.flushOK.Load(),
			FlushErr:       counters.flushErr.Load(),
			LastFlushMs:    counters.lastFlushMs.Load(),
			LastFlushBatch: counters.lastFlushBatch.Load(),
		}
	})

	metricsEndpoint := "/v1/ingest/metrics"
	healthEndpoint := "/v1/ingest/health"
	inventoryEndpoint := "/v1/ingest/inventory"
	bootstrapEndpoint := "/v1/ingest/bootstrap"
	networkCaptureEndpoint := "/v1/ingest/network_capture"

	metricsUC := usecase.NewCollectAndBufferWithIdentity(newFilteredCollector(collector, "sysmetrics", metricsEndpoint, 10*time.Second, metricsKeys()), outboxRepo, log, authHeader, identityHeader, metricsEndpoint)
	healthUC := usecase.NewCollectAndBufferWithIdentity(newFilteredCollector(collector, "sysmetrics_health", healthEndpoint, 10*time.Minute, healthKeys()), outboxRepo, log, authHeader, identityHeader, healthEndpoint)
	inventoryUC := usecase.NewCollectAndBufferWithIdentity(newFilteredCollector(collector, "sysmetrics_inventory", inventoryEndpoint, 8*time.Hour, inventoryKeys()), outboxRepo, log, authHeader, identityHeader, inventoryEndpoint)
	bootstrapUC := usecase.NewCollectAndBufferWithIdentity(newFilteredCollector(collector, "sysmetrics_bootstrap", bootstrapEndpoint, 24*time.Hour, bootstrapKeys()), outboxRepo, log, authHeader, identityHeader, bootstrapEndpoint)
	networkCaptureUC := usecase.NewCollectAndBufferWithIdentity(networkcapture.New(prefStore.Get), outboxRepo, log, authHeader, identityHeader, networkCaptureEndpoint)
	customMetricsUC := usecase.NewCollectAndBufferWithIdentity(custommetrics.New(cfg, prefStore.Get), outboxRepo, log, authHeader, identityHeader, metricsEndpoint)

	commandChan := make(chan usecase.ControlCommand, 10)
	configSyncUC := usecase.NewConfigSync(cfg, log, prefStore, commandChan)

	var pendingCfg *hub.PendingConfigStore
	onIngestConfig := func(auth string, cfg usecase.IngestConfig) {
		if cfg.Payload == nil {
			return
		}
		version, applied, err := usecase.ApplyConfigPayload(log, prefStore, commandChan, *cfg.Payload)
		if err != nil {
			log.Error(logger.KV("ingest config persist failed",
				"version", version,
				"err", err,
			))
			return
		}
		if applied {
			log.Info(logger.KV("ingest config applied",
				"version", version,
			))
		}
	}
	if mode == "hub" {
		pendingCfg = hub.NewPendingConfigStore()
		onIngestConfig = func(auth string, cfg usecase.IngestConfig) {
			if len(cfg.Raw) > 0 && auth != authHeader {
				pendingCfg.Set(auth, cfg.Raw)
			}
			if cfg.Payload == nil || auth != authHeader {
				return
			}
			version, applied, err := usecase.ApplyConfigPayload(log, prefStore, commandChan, *cfg.Payload)
			if err != nil {
				log.Error(logger.KV("ingest config persist failed",
					"version", version,
					"err", err,
				))
				return
			}
			if applied {
				log.Info(logger.KV("ingest config applied",
					"version", version,
				))
			}
		}
	}

	flushOptions := usecase.FlushOutboxOptions{BatchSize: cfg.OutboxFlushBatch}
	flushUC := usecase.NewFlushOutboxWithOptions(outboxRepo, tx, log, authHeader, onIngestConfig, flushOptions)
	pingUC := usecase.NewPingBackend(cfg, log)
	selfUpdateUC := usecase.NewSelfUpdate(cfg, log)
	controlClient := agentlessremote.NewAgentControlClient(cfg)
	var errorReportMu sync.Mutex
	lastErrorReportAt := make(map[string]time.Time)
	reportWorkerError := func(ctx context.Context, errorType, severity, recovery string, err error, metadata map[string]any) {
		if controlClient == nil {
			return
		}
		summary := ""
		if err != nil {
			summary = truncateTextForErrorReport(err.Error(), 900)
		}
		if severity == "" {
			severity = "error"
		}
		if recovery == "" {
			recovery = "open"
		}
		fingerprint := workerErrorFingerprint(errorType, mode, summary)
		now := time.Now()

		errorReportMu.Lock()
		lastAt := lastErrorReportAt[fingerprint]
		if !lastAt.IsZero() && now.Sub(lastAt) < 60*time.Second {
			errorReportMu.Unlock()
			return
		}
		lastErrorReportAt[fingerprint] = now
		errorReportMu.Unlock()

		event := entities.WorkerErrorEvent{
			Source:         "agent",
			ErrorType:      errorType,
			Severity:       severity,
			RecoveryStatus: recovery,
			Fingerprint:    fingerprint,
			Summary:        summary,
			Stack:          summary,
			Metadata:       metadata,
			OccurredAt:     now.UTC().Format(time.RFC3339),
		}
		if event.Metadata == nil {
			event.Metadata = map[string]any{}
		}
		event.Metadata["agent_mode"] = mode
		if reportErr := controlClient.ReportWorkerErrors(ctx, []entities.WorkerErrorEvent{event}); reportErr != nil {
			log.Error(logger.KV("worker error report failed",
				"route", "/v1/agent/error-report",
				"error_type", errorType,
				"err", reportErr,
			))
		}
	}

	if err := selfUpdateUC.ReportPendingResult(ctx); err != nil {
		log.Error(logger.KV("self update startup report failed",
			"err", err,
		))
		reportWorkerError(ctx, "self_update_startup_report_failed", "warning", "open", err, map[string]any{
			"route": "/v1/agent/update-report",
		})
	}
	var osLogCollectUC *usecase.CollectAndBuffer
	var osLogFlushUC *usecase.FlushOutbox
	var osLogRepo ports.OutboxRepo
	var agentlessUC *usecase.AgentlessHub
	var agentlessLastPoll time.Time
	var agentlessLastFlush time.Time
	var agentlessRepo ports.AgentlessOutboxRepo
	var agentlessBusy atomic.Bool
	// Inicializa coletor de logs do SO; o gating agora é feito pelas prefs (collect_logs e flags avançados).
	osStore := outbox.NewMemStore()
	osRepo := repositories.NewOutboxRepository(osStore)
	osLogRepo = osRepo
	osCollector := oslogs.New(cfg, prefStore.Get)
	osLogCollectUC = usecase.NewCollectAndBufferWithIdentity(osCollector, osRepo, log, authHeader, identityHeader, "/v1/logs/raw")
	var osTx ports.Transport
	if mode == "relay" {
		osTx = transport.NewHubClient(cfg)
	} else {
		osTx = transport.NewHTTPLogsClient(cfg)
	}
	osLogFlushUC = usecase.NewFlushOutboxWithOptions(osRepo, osTx, log, authHeader, onIngestConfig, flushOptions)
	pruneStore(osStore, "oslogs")

	agentlessSettings := func() usecase.AgentlessSettings {
		p := prefStore.Get()
		enabled := cfg.AgentlessEnabled
		// Após o primeiro config sync (prefs com versão), a web vira fonte de verdade.
		// Antes disso, mantém fallback de bootstrap por env para compatibilidade.
		if strings.TrimSpace(p.Version) != "" {
			enabled = p.AgentlessEnabled
		} else if p.AgentlessEnabled {
			enabled = true
		}
		pollSec := p.AgentlessPollSec
		if pollSec <= 0 {
			pollSec = int(cfg.AgentlessPollInterval.Seconds())
		}
		flushSec := p.AgentlessFlushSec
		if flushSec <= 0 {
			flushSec = int(cfg.AgentlessFlushInterval.Seconds())
		}
		jobsLimit := p.AgentlessJobsLimit
		if jobsLimit <= 0 {
			jobsLimit = cfg.AgentlessJobsLimit
		}
		lockSec := p.AgentlessLockSec
		if lockSec <= 0 {
			lockSec = cfg.AgentlessLockSec
		}
		flushBatch := p.AgentlessFlushBatch
		if flushBatch <= 0 {
			flushBatch = cfg.AgentlessFlushBatch
		}
		return usecase.AgentlessSettings{
			Enabled:    enabled,
			PollSec:    pollSec,
			FlushSec:   flushSec,
			JobsLimit:  jobsLimit,
			LockSec:    lockSec,
			FlushBatch: flushBatch,
		}
	}

	// Inicializa worker agentless em todos os modos não-relay.
	// A web (prefs remotas) continua sendo a fonte de verdade para enable/disable em runtime.
	if mode != "relay" {
		agentlessStore := selectAgentlessOutbox(cfg, log)
		agentlessRepo = repositories.NewAgentlessOutboxRepository(agentlessStore)
		agentlessClient := agentlessremote.NewAgentlessHubClient(cfg)
		targetsStore := selectAgentlessTargetsStore(log)
		agentlessUC = usecase.NewAgentlessHub(cfg, log, agentlessClient, agentlessRepo, agentlessSettings, targetsStore)
		agentlessLastPoll = time.Now().Add(-cfg.AgentlessPollInterval)
		agentlessLastFlush = time.Now().Add(-cfg.AgentlessFlushInterval)
	}

	modeApplier := modechange.NewApplier(cfg, log)
	selfHealExec := usecase.NewSelfHealExecutor(log, controlClient, usecase.SelfHealDeps{
		ConfigSync:       configSyncUC.Execute,
		Ping:             pingUC.Execute,
		ApplyAgentMode:   modeApplier.Apply,
		CollectMetrics:   metricsUC.Execute,
		CollectHealth:    healthUC.Execute,
		CollectInventory: inventoryUC.Execute,
		CollectBootstrap: bootstrapUC.Execute,
		CollectNetwork:   networkCaptureUC.Execute,
		AgentlessSync: func(runCtx context.Context) error {
			if agentlessUC == nil {
				return nil
			}
			return agentlessUC.SyncTargets(runCtx)
		},
		AgentlessCollectNow: func(runCtx context.Context) {
			if agentlessUC != nil {
				agentlessUC.CollectNow(runCtx)
			}
		},
		AgentlessFlush: func(runCtx context.Context) error {
			if agentlessUC == nil {
				return nil
			}
			return agentlessUC.Flush(runCtx)
		},
		ClearAgentlessLock: func() {
			agentlessBusy.Store(false)
		},
		HasAgentlessWorker: func() bool {
			return agentlessUC != nil
		},
		RuntimeSnapshot: func() map[string]any {
			p := prefStore.Get()
			settings := agentlessSettings()
			return buildSelfHealRuntimeSnapshot(cfg, mode, p, settings, agentlessUC != nil, selfUpdateUC)
		},
	})
	commandDedupe := usecase.NewCommandIdempotency(6*time.Hour, 2048)
	var activeChannelClient *usecase.AgentChannelClient

	if cfg.HealthPort > 0 {
		go health.Serve(cfg.HealthPort, log, func() health.Snapshot {
			items, bytes := outboxRepo.Len()
			if osLogRepo != nil {
				oi, ob := osLogRepo.Len()
				items += oi
				bytes += ob
			}
			procRSS, procCPU := procStats(proc)
			var channelStatus any
			if activeChannelClient != nil {
				channelStatus = activeChannelClient.Snapshot()
			}
			return health.Snapshot{
				Status:           "ok",
				PipelineVersion:  agentruntime.PipelineVersion,
				QueueItems:       items,
				QueueBytes:       bytes,
				FlushOK:          counters.flushOK.Load(),
				FlushErr:         counters.flushErr.Load(),
				CollectErr:       counters.collectErr.Load(),
				InvalidEnv:       metrics.InvalidEnvelopesTotal(),
				AgentlessJobs:    metrics.AgentlessJobsTotal(),
				UptimeSec:        int64(time.Since(startedAt).Seconds()),
				ProcRSS:          procRSS,
				ProcCPU:          procCPU,
				Goroutines:       runtime.NumGoroutine(),
				LastCollectMs:    counters.lastCollectMs.Load(),
				LastFlushMs:      counters.lastFlushMs.Load(),
				LastFlushBatch:   counters.lastFlushBatch.Load(),
				IngestTimeoutSec: int64(cfg.IngestTimeout.Seconds()),
				FlushIntervalSec: int64(cfg.OutboxFlushInterval.Seconds()),
				FlushBatchLimit:  cfg.OutboxFlushBatch,
				FlushDetail:      flushUC.Snapshot(),
				Channel:          channelStatus,
			}
		})
	}

	if mode == "hub" {
		addr := cfg.HubListenAddr
		if addr == "" {
			addr = ":9090"
		}
		go hub.ServeHub(addr, cfg, outboxRepo, log, pendingCfg)
	}

	tMetrics := time.NewTicker(10 * time.Second)
	tHealth := time.NewTicker(10 * time.Minute)
	tInventory := time.NewTicker(8 * time.Hour)
	tFlush := time.NewTicker(cfg.OutboxFlushInterval)
	var tPing *time.Ticker
	var tCfgSync *time.Ticker
	var tOsCollect *time.Ticker
	var tCustomMetrics *time.Ticker
	var tAgentlessTick *time.Ticker
	var tSelfHeal *time.Ticker
	tPing = time.NewTicker(cfg.PingInterval)
	tCfgSync = time.NewTicker(cfg.ConfigSyncInterval)
	if osLogCollectUC != nil {
		tOsCollect = time.NewTicker(cfg.OSLogInterval)
	}
	tCustomMetrics = time.NewTicker(cfg.CustomMetricsInterval)
	if agentlessUC != nil {
		tAgentlessTick = time.NewTicker(5 * time.Second)
	}
	tSelfHeal = time.NewTicker(cfg.SelfHealPollInterval)
	defer tMetrics.Stop()
	defer tHealth.Stop()
	defer tInventory.Stop()
	defer tFlush.Stop()
	if tPing != nil {
		defer tPing.Stop()
	}
	if tCfgSync != nil {
		defer tCfgSync.Stop()
	}
	if tOsCollect != nil {
		defer tOsCollect.Stop()
	}
	if tCustomMetrics != nil {
		defer tCustomMetrics.Stop()
	}
	if tAgentlessTick != nil {
		defer tAgentlessTick.Stop()
	}
	if tSelfHeal != nil {
		defer tSelfHeal.Stop()
	}

	log.Info(logger.KV("agent started",
		"mode", mode,
	))
	if mode == "direct" || mode == "hub" || mode == "relay" {
		channelClient := usecase.NewAgentChannelClient(cfg, log)
		activeChannelClient = channelClient
		channelClient.SetCommandHandler(func(runCtx context.Context, envelope channel.Envelope) error {
			code := channelEnvelopeCode(envelope)
			if !channel.IsAllowedCommandCode(code) || channel.IsShellLikeCommandCode(code) {
				reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeError, map[string]any{
					"status":        "failed",
					"stage":         "security",
					"failure_class": "command_not_allowed",
					"error":         "command blocked by allowlist",
				})
				log.Error(logger.KV("channel command blocked",
					"command_id", strings.TrimSpace(envelope.CommandID),
					"command_code", code,
					"err", "command_not_allowed",
				))
				return nil
			}
			if code == channel.CommandCollectNow {
				commandID := strings.TrimSpace(envelope.CommandID)
				if commandID == "" {
					return nil
				}
				if !commandDedupe.First(commandID) {
					reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeAck, map[string]any{
						"status": "duplicate",
						"stage":  "dedupe",
					})
					return nil
				}
				collectNow := channelEnvelopeCollectNow(envelope)
				reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeAck, map[string]any{
					"status":      "accepted",
					"stage":       "ack",
					"collect_now": collectNow,
				})
				if len(collectNow) == 0 {
					reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeError, map[string]any{
						"status": "failed",
						"stage":  "precheck",
						"error":  "empty collect_now",
					})
					return nil
				}
				agentlessCommand := channelEnvelopeAgentlessCommand(envelope)
				for _, name := range collectNow {
					cmd := usecase.ControlCommand{
						Name:          name,
						CommandID:     commandID,
						CorrelationID: strings.TrimSpace(envelope.CorrelationID),
						Source:        "channel",
					}
					if name == "agentless" {
						cmd.CheckIDs = agentlessCommand.CheckIDs
						cmd.TimeoutMs = agentlessCommand.TimeoutMs
						if agentlessCommand.CommandID != "" {
							cmd.CommandID = agentlessCommand.CommandID
						}
						if agentlessCommand.CorrelationID != "" {
							cmd.CorrelationID = agentlessCommand.CorrelationID
						}
					}
					select {
					case commandChan <- cmd:
						progress := map[string]any{
							"status": "queued",
							"stage":  "queued",
							"name":   name,
						}
						if len(cmd.CheckIDs) > 0 {
							progress["check_ids"] = cmd.CheckIDs
						}
						reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeProgress, progress)
					default:
						reportChannelCommandEvent(runCtx, channelClient, envelope, channel.TypeError, map[string]any{
							"status":        "failed",
							"stage":         "backpressure",
							"name":          name,
							"failure_class": "backpressure",
						})
					}
				}
				return nil
			}
			result := usecase.ExecuteChannelSelfHealCommand(runCtx, commandDedupe, selfHealExec, channelClient, envelope)
			if !result.Executed {
				log.Info(logger.KV("channel command duplicate skipped",
					"command_id", strings.TrimSpace(envelope.CommandID),
					"status", result.Status,
					"message", result.Message,
				))
				return nil
			}
			if result.Status == "failed" || result.Status == channel.StatusTimeout {
				code := channelEnvelopeCode(envelope)
				reportWorkerError(runCtx, "channel_selfheal_command_failed", "warning", "open", errors.New(result.Message), map[string]any{
					"command_id":     strings.TrimSpace(envelope.CommandID),
					"command_code":   code,
					"correlation_id": strings.TrimSpace(envelope.CorrelationID),
					"evidence":       result.Evidence,
				})
			}
			return nil
		})
		go channelClient.Run(ctx)
	}
	reportControlCollectProgress := func(runCtx context.Context, cmd usecase.ControlCommand, stage, name string, extra map[string]any) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		progress := map[string]any{
			"status": "running",
			"stage":  stage,
			"name":   name,
			"source": strings.TrimSpace(cmd.Source),
		}
		if stage == "completed" {
			progress["status"] = "success"
		}
		for k, v := range extra {
			progress[k] = v
		}
		_ = activeChannelClient.SendEvent(runCtx, channel.Envelope{
			Type:          channel.TypeProgress,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Progress:      progress,
		})
	}
	reportControlCollectError := func(runCtx context.Context, cmd usecase.ControlCommand, name string, err error) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		_ = activeChannelClient.SendEvent(runCtx, channel.Envelope{
			Type:          channel.TypeError,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Error: map[string]any{
				"status":        "failed",
				"stage":         "collect",
				"name":          name,
				"failure_class": "collect_failed",
				"message":       errString(err),
			},
		})
	}
	reportControlCollectTimeout := func(runCtx context.Context, cmd usecase.ControlCommand, name string) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		_ = activeChannelClient.SendEvent(runCtx, channel.Envelope{
			Type:          channel.TypeTimeout,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			TimeoutMs:     cmd.TimeoutMs,
			Error: map[string]any{
				"status":        "timeout",
				"stage":         "collect",
				"name":          name,
				"scope":         agentlessCommandScope(cmd),
				"check_ids":     cmd.CheckIDs,
				"failure_class": "timeout",
				"message":       "agentless command timeout",
			},
		})
	}
	reportControlCollectBusy := func(runCtx context.Context, cmd usecase.ControlCommand, name string) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		_ = activeChannelClient.SendEvent(runCtx, channel.Envelope{
			Type:          channel.TypeError,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Error: map[string]any{
				"status":        "skipped_busy",
				"stage":         "precheck",
				"name":          name,
				"scope":         agentlessCommandScope(cmd),
				"check_ids":     cmd.CheckIDs,
				"failure_class": "agentless_busy",
				"reason":        "busy",
				"message":       "agentless collector busy",
			},
		})
	}
	reportControlAgentlessResult := func(runCtx context.Context, cmd usecase.ControlCommand) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		_ = activeChannelClient.SendEvent(runCtx, channel.Envelope{
			Type:          channel.TypeResult,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Result: map[string]any{
				"status":    "success",
				"stage":     "completed",
				"name":      "agentless",
				"scope":     agentlessCommandScope(cmd),
				"check_ids": cmd.CheckIDs,
				"fallback":  len(cmd.CheckIDs) == 0,
			},
		})
	}
	reportControlCollectResult := func(runCtx context.Context, cmd usecase.ControlCommand, result *usecase.BufferedCollectResult) {
		if activeChannelClient == nil || strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		sendCollectChunks(runCtx, activeChannelClient, cmd, result)
	}

	// coleta de bootstrap imediata (host/inventory estático)
	if err := bootstrapUC.Execute(ctx); err != nil {
		reportWorkerError(ctx, "collect_bootstrap_failed", "warning", "open", err, map[string]any{
			"route": bootstrapEndpoint,
		})
	}

	for {
		select {
		case <-ctx.Done():
			log.Info(logger.KV("shutdown"))
			return nil
		case <-tMetrics.C:
			start := time.Now()
			if err := metricsUC.Execute(ctx); err != nil {
				counters.collectErr.Add(1)
				reportWorkerError(ctx, "collect_metrics_failed", "error", "open", err, map[string]any{
					"route": metricsEndpoint,
				})
			}
			counters.lastCollectMs.Store(time.Since(start).Milliseconds())
		case <-readTick(tCustomMetrics):
			if err := customMetricsUC.Execute(ctx); err != nil {
				counters.collectErr.Add(1)
				reportWorkerError(ctx, "collect_custom_metrics_failed", "warning", "open", err, map[string]any{
					"route": metricsEndpoint,
				})
			}
		case <-tHealth.C:
			if err := healthUC.Execute(ctx); err != nil {
				reportWorkerError(ctx, "collect_health_failed", "warning", "open", err, map[string]any{
					"route": healthEndpoint,
				})
			}
		case <-tInventory.C:
			if err := inventoryUC.Execute(ctx); err != nil {
				reportWorkerError(ctx, "collect_inventory_failed", "warning", "open", err, map[string]any{
					"route": inventoryEndpoint,
				})
			}
		case <-tFlush.C:
			pruneStore(store, "main")
			pruneStore(osStore, "oslogs")
			start := time.Now()
			if n, err := flushUC.Execute(ctx); err != nil {
				counters.flushErr.Add(1)
				reportWorkerError(ctx, "flush_outbox_failed", "error", "open", err, map[string]any{
					"route": "/v1/ingest",
				})
			} else {
				counters.flushOK.Add(1)
				counters.lastFlushBatch.Store(int64(n))
			}
			counters.lastFlushMs.Store(time.Since(start).Milliseconds())
			if osLogFlushUC != nil {
				start := time.Now()
				if n, err := osLogFlushUC.Execute(ctx); err != nil {
					counters.flushErr.Add(1)
					reportWorkerError(ctx, "flush_oslogs_failed", "warning", "open", err, map[string]any{
						"route": "/v1/logs/raw",
					})
				} else {
					counters.flushOK.Add(1)
					counters.lastFlushBatch.Store(int64(n))
				}
				counters.lastFlushMs.Store(time.Since(start).Milliseconds())
			}
		case <-readTick(tPing):
			if err := pingUC.Execute(ctx); err != nil {
				reportWorkerError(ctx, "api_connectivity_ping_failed", "warning", "open", err, map[string]any{
					"route": "/v1/agent/ping",
				})
			}
		case <-readTick(tCfgSync):
			if err := configSyncUC.Execute(ctx); err != nil {
				reportWorkerError(ctx, "config_sync_failed", "warning", "open", err, map[string]any{
					"route": "/v1/agent/config",
				})
			}
		case cmd := <-commandChan:
			switch cmd.Name {
			case "inventory":
				reportControlCollectProgress(ctx, cmd, "running", "inventory", nil)
				result, err := inventoryUC.ExecuteDetailed(ctx)
				if err != nil {
					reportControlCollectError(ctx, cmd, "inventory", err)
					reportWorkerError(ctx, "collect_inventory_failed", "warning", "open", err, map[string]any{
						"source": "command",
						"name":   cmd.Name,
					})
				} else {
					reportControlCollectResult(ctx, cmd, result)
				}
			case "health":
				reportControlCollectProgress(ctx, cmd, "running", "health", nil)
				result, err := healthUC.ExecuteDetailed(ctx)
				if err != nil {
					reportControlCollectError(ctx, cmd, "health", err)
					reportWorkerError(ctx, "collect_health_failed", "warning", "open", err, map[string]any{
						"source": "command",
						"name":   cmd.Name,
					})
				} else {
					reportControlCollectResult(ctx, cmd, result)
				}
			case "bootstrap":
				reportControlCollectProgress(ctx, cmd, "running", "bootstrap", nil)
				result, err := bootstrapUC.ExecuteDetailed(ctx)
				if err != nil {
					reportControlCollectError(ctx, cmd, "bootstrap", err)
					reportWorkerError(ctx, "collect_bootstrap_failed", "warning", "open", err, map[string]any{
						"source": "command",
						"name":   cmd.Name,
					})
				} else {
					reportControlCollectResult(ctx, cmd, result)
				}
			case "network_capture":
				reportControlCollectProgress(ctx, cmd, "running", "network_capture", nil)
				result, err := networkCaptureUC.ExecuteDetailed(ctx)
				if err != nil {
					reportControlCollectError(ctx, cmd, "network_capture", err)
					reportWorkerError(ctx, "collect_network_capture_failed", "warning", "open", err, map[string]any{
						"source": "command",
						"name":   cmd.Name,
					})
				} else {
					reportControlCollectResult(ctx, cmd, result)
				}
			case "agentless":
				if agentlessUC != nil {
					if agentlessBusy.CompareAndSwap(false, true) {
						cmdCopy := cmd
						go func(commandName string, cmd usecase.ControlCommand) {
							defer agentlessBusy.Store(false)
							runCtx := ctx
							cancel := func() {}
							if cmd.TimeoutMs > 0 {
								runCtx, cancel = context.WithTimeout(ctx, time.Duration(cmd.TimeoutMs)*time.Millisecond)
							}
							defer cancel()
							reportControlCollectProgress(runCtx, cmd, "running", "agentless", map[string]any{
								"scope":     agentlessCommandScope(cmd),
								"check_ids": cmd.CheckIDs,
							})
							if len(cmd.CheckIDs) > 0 {
								err := agentlessUC.CollectCommand(runCtx, usecase.AgentlessCommandRequest{
									CommandID:     cmd.CommandID,
									CorrelationID: cmd.CorrelationID,
									CheckIDs:      cmd.CheckIDs,
									TimeoutMs:     cmd.TimeoutMs,
								})
								if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
									reportControlCollectTimeout(ctx, cmd, "agentless")
									return
								}
								if err != nil {
									reportControlCollectError(runCtx, cmd, "agentless", err)
									log.Error(logger.KV("agentless command collect failed",
										"command_id", cmd.CommandID,
										"correlation_id", cmd.CorrelationID,
										"err", err,
									))
									reportWorkerError(ctx, "agentless_collect_failed", "error", "open", err, map[string]any{
										"source":         "command",
										"name":           commandName,
										"command_id":     cmd.CommandID,
										"correlation_id": cmd.CorrelationID,
									})
									return
								}
							} else {
								if err := agentlessUC.SyncTargets(runCtx); err != nil {
									reportControlCollectError(runCtx, cmd, "agentless", err)
									log.Error(logger.KV("agentless targets sync failed",
										"err", err,
									))
									reportWorkerError(ctx, "agentless_sync_failed", "error", "open", err, map[string]any{
										"source": "command",
										"name":   commandName,
									})
								}
								agentlessUC.CollectNow(runCtx)
							}
							if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
								reportControlCollectTimeout(ctx, cmd, "agentless")
								return
							}
							reportControlAgentlessResult(ctx, cmd)
						}(cmdCopy.Name, cmdCopy)
					} else {
						log.Info(logger.KV("agentless command skipped",
							"source", "command",
							"name", cmd.Name,
							"reason", "busy",
						))
						reportControlCollectBusy(ctx, cmd, "agentless")
					}
				}
			case "self_update":
				if err := selfUpdateUC.Execute(ctx, cmd.Update); err != nil {
					ver := ""
					if cmd.Update != nil {
						ver = cmd.Update.Version
					}
					log.Error(logger.KV("self update failed",
						"version", ver,
						"err", err,
					))
					reportWorkerError(ctx, "self_update_apply_failed", "error", "open", err, map[string]any{
						"version": ver,
					})
				}
			case "self_update_policy":
				selfUpdateUC.ApplyRemoteConfig(cmd.AutoUpdate)
			}
		case <-readTick(tOsCollect):
			if osLogCollectUC != nil {
				if err := osLogCollectUC.Execute(ctx); err != nil {
					reportWorkerError(ctx, "collect_oslogs_failed", "warning", "open", err, map[string]any{
						"route": "/v1/logs/raw",
					})
				}
			}
		case <-readTick(tAgentlessTick):
			if agentlessUC != nil {
				st := agentlessSettings()
				now := time.Now()
				shouldPoll := st.Enabled && (agentlessLastPoll.IsZero() || now.Sub(agentlessLastPoll) >= time.Duration(st.PollSec)*time.Second)
				shouldFlush := agentlessLastFlush.IsZero() || now.Sub(agentlessLastFlush) >= time.Duration(st.FlushSec)*time.Second
				if (shouldPoll || shouldFlush) && agentlessBusy.CompareAndSwap(false, true) {
					if shouldPoll {
						agentlessLastPoll = now
					}
					if shouldFlush {
						agentlessLastFlush = now
					}
					go func(runPoll, runFlush bool) {
						defer agentlessBusy.Store(false)
						if runPoll {
							if err := agentlessUC.SyncTargets(ctx); err != nil {
								log.Error(logger.KV("agentless targets sync failed",
									"err", err,
								))
								reportWorkerError(ctx, "agentless_sync_failed", "error", "open", err, map[string]any{
									"source": "ticker",
									"name":   "agentless_poll",
								})
							}
							if err := agentlessUC.PollAndRun(ctx); err != nil {
								reportWorkerError(ctx, "agentless_collect_failed", "error", "open", err, map[string]any{
									"source": "ticker",
									"name":   "agentless_poll",
								})
							}
						}
						if runFlush {
							if err := agentlessUC.Flush(ctx); err != nil {
								reportWorkerError(ctx, "agentless_flush_failed", "error", "open", err, map[string]any{
									"source": "ticker",
									"name":   "agentless_flush",
								})
							}
						}
					}(shouldPoll, shouldFlush)
				}
			}
		case <-readTick(tSelfHeal):
			commands, err := controlClient.FetchSelfHealCommands(ctx)
			if err != nil {
				reportWorkerError(ctx, "selfheal_commands_pull_failed", "warning", "open", err, map[string]any{
					"route": "/v1/agent/selfheal-commands",
				})
				break
			}
			if len(commands) > 0 {
				log.Info(logger.KV("selfheal commands fetched",
					"count", len(commands),
				))
			}
			for _, command := range commands {
				status, message, evidence, executed := usecase.ExecuteSelfHealOnce(ctx, commandDedupe, selfHealExec, command)
				if !executed {
					log.Info(logger.KV("selfheal command duplicate skipped",
						"command_id", strings.TrimSpace(command.CommandID),
					))
					continue
				}
				if status == "failed" {
					reportWorkerError(ctx, "selfheal_command_failed", "warning", "open", errors.New(message), map[string]any{
						"command_id":     strings.TrimSpace(command.CommandID),
						"command_code":   strings.TrimSpace(command.Code),
						"correlation_id": strings.TrimSpace(command.CorrelationID),
						"evidence":       evidence,
					})
				}
			}
		}
	}
}

func bootstrap(ctx context.Context, cfg config.Config, log logger.Logger) error {
	if cfg.Agent.Token == "" {
		return errors.New("missing agent token")
	}
	hi, _ := host.InfoWithContext(ctx)
	hostname, _ := os.Hostname()

	// Se já existe estado persistido com mesmo token/host, pula bootstrap.
	if st, err := loadBootstrapState(); err == nil {
		if st.Token == cfg.Agent.Token {
			log.Info(logger.KV("bootstrap skipped",
				"reason", "state_found",
			))
			return nil
		}
		return errors.New("bootstrap state mismatch: token diferente; limpe data/bootstrap.ok se deseja revalidar")
	}

	payload := map[string]any{
		"token":            cfg.Agent.Token,
		"hostname":         hostname,
		"os":               hi.OS,
		"platform":         hi.Platform,
		"platform_version": hi.PlatformVersion,
		"arch":             runtime.GOARCH,
		"ip_instalacao":    firstIP(),
		"host_guid":        hi.HostID,
		"versao_agente":    version.Version,
	}
	if identity := cfg.AgentIdentityClaim(hi.HostID); len(identity) > 0 {
		payload["agent_identity"] = identity
	}
	body, _ := json.Marshal(payload)
	url := cfg.APIEndpoint("/v1/agent/bootstrap")
	if cfg.AgentMode == "relay" && cfg.HubURL != "" {
		url = cfg.HubURL + "/v1/agent/bootstrap"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build bootstrap request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+cfg.Agent.Token)
	if identityHeader := cfg.AgentIdentityHeader(hi.HostID); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}

	cl := httpx.NewClient(cfg, 10*time.Second)
	resp, err := cl.Do(req)
	if err != nil {
		return fmt.Errorf("bootstrap http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bootstrap rejected: %s body=%s", resp.Status, string(respBody))
	}
	_ = persistToken(cfg.Agent.Token)
	_ = persistBootstrapState(cfg.Agent.Token, hi.HostID)
	log.Info(logger.KV("bootstrap ok"))
	return nil
}

func firstIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}
		if (iface.Flags & net.FlagLoopback) != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

// selectOutbox escolhe entre bbolt (persistente) ou memória.
func selectOutbox(cfg config.Config, log logger.Logger) repositories.Store {
	maxMB := cfg.OutboxMaxMB
	path := cfg.OutboxPath
	if path != "" {
		if bs, err := outbox.NewBoltStore(path, maxMB); err == nil {
			log.Info(logger.KV("outbox bolt enabled",
				"path", path,
			))
			return bs
		} else {
			log.Error(logger.KV("outbox bolt fallback to memory",
				"err", err,
			))
		}
	}
	log.Info(logger.KV("outbox memory enabled"))
	return outbox.NewMemStore()
}

func selectAgentlessOutbox(cfg config.Config, log logger.Logger) repositories.AgentlessStore {
	maxMB := cfg.AgentlessOutboxMaxMB
	path := cfg.AgentlessOutboxPath
	if path != "" {
		if bs, err := agentlessstore.NewBoltStore(path, maxMB); err == nil {
			log.Info(logger.KV("agentless outbox bolt enabled",
				"path", path,
			))
			return bs
		} else {
			log.Error(logger.KV("agentless outbox bolt fallback to memory",
				"err", err,
			))
		}
	}
	log.Info(logger.KV("agentless outbox memory enabled"))
	return agentlessstore.NewMemStore()
}

func selectAgentlessTargetsStore(log logger.Logger) *agentlessstore.TargetsStore {
	path := os.Getenv("AGENTLESS_TARGETS_PATH")
	if path == "" {
		path = "./data/agentless_targets.json"
	}
	log.Info(logger.KV("agentless targets store enabled",
		"path", path,
	))
	return agentlessstore.NewTargetsStore(path)
}

type obsCounters struct {
	flushOK        atomic.Int64
	flushErr       atomic.Int64
	collectErr     atomic.Int64
	lastCollectMs  atomic.Int64
	lastFlushMs    atomic.Int64
	lastFlushBatch atomic.Int64
}

func processHandle() *ps.Process {
	p, err := ps.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil
	}
	return p
}

func procStats(p *ps.Process) (rss int64, cpu float64) {
	if p == nil {
		return 0, 0
	}
	if mi, err := p.MemoryInfo(); err == nil {
		rss = int64(mi.RSS)
	}
	if pct, err := p.CPUPercent(); err == nil {
		cpu = pct
	}
	return rss, cpu
}

func persistToken(token string) error {
	path := os.Getenv("AGENT_TOKEN_PATH")
	if path == "" {
		path = "./data/agent.token"
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return os.WriteFile(path, []byte(token), 0o600)
}

type bootstrapState struct {
	Token    string `json:"token"`
	HostGUID string `json:"host_guid,omitempty"`
}

func persistBootstrapState(token, hostGUID string) error {
	path := os.Getenv("AGENT_STATE_PATH")
	if path == "" {
		path = "./data/bootstrap.ok"
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	payload, _ := json.Marshal(bootstrapState{Token: token, HostGUID: hostGUID})
	return os.WriteFile(path, payload, 0o600)
}

func loadBootstrapState() (bootstrapState, error) {
	path := os.Getenv("AGENT_STATE_PATH")
	if path == "" {
		path = "./data/bootstrap.ok"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return bootstrapState{}, err
	}
	var st bootstrapState
	if err := json.Unmarshal(b, &st); err != nil {
		return bootstrapState{}, err
	}
	return st, nil
}

// filteredCollector reusa o coletor base mas mantém apenas chaves do body relevantes para cada endpoint.
type filteredCollector struct {
	base     ports.Collector
	name     string
	endpoint string
	interval time.Duration
	keep     map[string]struct{}
}

func newFilteredCollector(base ports.Collector, name, endpoint string, interval time.Duration, keys map[string]struct{}) ports.Collector {
	return &filteredCollector{
		base:     base,
		name:     name,
		endpoint: endpoint,
		interval: interval,
		keep:     keys,
	}
}

func (f *filteredCollector) Name() string            { return f.name }
func (f *filteredCollector) Interval() time.Duration { return f.interval }
func (f *filteredCollector) Collect(ctx context.Context) ([]byte, error) {
	raw, err := f.base.Collect(ctx)
	if err != nil || raw == nil || len(raw) == 0 {
		return raw, err
	}
	if len(f.keep) == 0 {
		return raw, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return raw, nil // fallback: não filtra se não conseguir parsear
	}
	for k := range body {
		if _, ok := f.keep[k]; !ok {
			delete(body, k)
		}
	}
	return json.Marshal(body)
}

func setOf(keys ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		m[k] = struct{}{}
	}
	return m
}

func metricsKeys() map[string]struct{} {
	return setOf(
		"capabilities", "cpu", "memory", "disk", "network", "net_active",
		"sanity", "services", "processes", "agent", "gpu", "power",
		"sensors", "time_sync",
	)
}

func healthKeys() map[string]struct{} {
	return setOf(
		"capabilities", "disk", "updates", "time_sync", "sanity", "vulns", "logs",
	)
}

func inventoryKeys() map[string]struct{} {
	return setOf(
		"capabilities", "inventory", "host", "agent",
	)
}

func bootstrapKeys() map[string]struct{} {
	return setOf(
		"capabilities", "inventory", "host", "network",
	)
}

func readTick(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}

func channelEnvelopeCode(env channel.Envelope) string {
	if env.Payload == nil {
		return ""
	}
	if value, ok := env.Payload["code"].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func channelEnvelopeCollectNow(env channel.Envelope) []string {
	if env.Payload == nil {
		return nil
	}
	if strings.TrimSpace(fmt.Sprint(env.Payload["scope"])) == "agentless_check" {
		return []string{"agentless"}
	}
	raw, ok := env.Payload["collect_now"]
	if !ok {
		raw = env.Payload["collect"]
	}
	allowed := map[string]struct{}{
		"inventory":       {},
		"health":          {},
		"bootstrap":       {},
		"agentless":       {},
		"network_capture": {},
	}
	out := []string{}
	appendName := func(value string) {
		normalized := strings.TrimSpace(value)
		if _, ok := allowed[normalized]; ok {
			out = append(out, normalized)
		}
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			appendName(value)
		}
	case []any:
		for _, value := range values {
			if text, ok := value.(string); ok {
				appendName(text)
			}
		}
	case string:
		appendName(values)
	}
	return out
}

func channelEnvelopeAgentlessCommand(env channel.Envelope) usecase.AgentlessCommandRequest {
	req := usecase.AgentlessCommandRequest{
		CommandID:     strings.TrimSpace(env.CommandID),
		CorrelationID: strings.TrimSpace(env.CorrelationID),
	}
	if env.Payload == nil {
		return req
	}
	if commandID := strings.TrimSpace(fmt.Sprint(env.Payload["command_id"])); commandID != "" && commandID != "<nil>" {
		req.CommandID = commandID
	}
	if correlationID := strings.TrimSpace(fmt.Sprint(env.Payload["correlation_id"])); correlationID != "" && correlationID != "<nil>" {
		req.CorrelationID = correlationID
	}
	req.CheckIDs = channelPayloadIntList(env.Payload["check_ids"])
	req.TimeoutMs = intFromAny(env.Payload["timeout_ms"])
	return req
}

func agentlessCommandScope(cmd usecase.ControlCommand) string {
	if len(cmd.CheckIDs) > 0 {
		return "agentless_check"
	}
	return "agentless"
}

func channelPayloadIntList(raw any) []int {
	out := []int{}
	seen := map[int]struct{}{}
	appendID := func(value int) {
		if value <= 0 {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	switch values := raw.(type) {
	case []int:
		for _, value := range values {
			appendID(value)
		}
	case []any:
		for _, value := range values {
			appendID(intFromAny(value))
		}
	case string:
		for _, part := range strings.Split(values, ",") {
			appendID(intFromAny(strings.TrimSpace(part)))
		}
	case float64:
		appendID(int(values))
	case int:
		appendID(values)
	}
	return out
}

func intFromAny(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

func reportChannelCommandEvent(ctx context.Context, client *usecase.AgentChannelClient, source channel.Envelope, eventType string, body map[string]any) {
	if client == nil {
		return
	}
	env := channel.Envelope{
		Type:          eventType,
		CommandID:     strings.TrimSpace(source.CommandID),
		CorrelationID: strings.TrimSpace(source.CorrelationID),
	}
	switch eventType {
	case channel.TypeAck:
		env.Payload = body
	case channel.TypeProgress:
		env.Progress = body
	case channel.TypeResult:
		env.Result = body
	case channel.TypeError:
		env.Error = body
	default:
		env.Payload = body
	}
	_ = client.SendEvent(ctx, env)
}

func sendCollectChunks(ctx context.Context, client *usecase.AgentChannelClient, cmd usecase.ControlCommand, result *usecase.BufferedCollectResult) {
	if client == nil || strings.TrimSpace(cmd.CommandID) == "" {
		return
	}
	if result == nil {
		_ = client.SendEvent(ctx, channel.Envelope{
			Type:          channel.TypeResult,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Result: map[string]any{
				"status": "success",
				"stage":  "completed",
				"name":   strings.TrimSpace(cmd.Name),
				"empty":  true,
			},
		})
		return
	}

	const chunkSize = 48 * 1024
	const maxChunks = 16
	body := result.Body
	totalSize := len(body)
	totalChunks := 0
	if totalSize > 0 {
		totalChunks = (totalSize + chunkSize - 1) / chunkSize
	}
	sentChunks := totalChunks
	truncated := false
	if sentChunks > maxChunks {
		sentChunks = maxChunks
		truncated = true
	}
	sum := sha1.Sum(body)
	sha := fmt.Sprintf("%x", sum)
	for i := 0; i < sentChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > totalSize {
			end = totalSize
		}
		_ = client.SendEvent(ctx, channel.Envelope{
			Type:          channel.TypeProgress,
			CommandID:     strings.TrimSpace(cmd.CommandID),
			CorrelationID: strings.TrimSpace(cmd.CorrelationID),
			Progress: map[string]any{
				"status": "running",
				"stage":  "chunk",
				"name":   result.Collector,
				"chunk": map[string]any{
					"index":           i + 1,
					"total":           totalChunks,
					"sent_total":      sentChunks,
					"size_bytes":      end - start,
					"payload_sha1":    sha,
					"encoding":        "base64",
					"data":            base64.StdEncoding.EncodeToString(body[start:end]),
					"truncated":       truncated,
					"max_chunk_bytes": chunkSize,
					"max_chunks":      maxChunks,
				},
			},
		})
	}
	_ = client.SendEvent(ctx, channel.Envelope{
		Type:          channel.TypeResult,
		CommandID:     strings.TrimSpace(cmd.CommandID),
		CorrelationID: strings.TrimSpace(cmd.CorrelationID),
		Result: map[string]any{
			"status":           "success",
			"stage":            "completed",
			"name":             result.Collector,
			"event_id":         result.EventID,
			"endpoint":         result.Endpoint,
			"duration_ms":      result.DurationMs,
			"payload_bytes":    totalSize,
			"payload_sha1":     sha,
			"chunks_total":     totalChunks,
			"chunks_sent":      sentChunks,
			"chunks_truncated": truncated,
			"fallback":         "ingest_outbox",
		},
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func truncateTextForErrorReport(text string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= maxLen {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:maxLen])
}

func workerErrorFingerprint(errorType, mode, summary string) string {
	seed := strings.TrimSpace(errorType) + "|" + strings.TrimSpace(mode) + "|" + strings.TrimSpace(summary)
	hash := sha1.Sum([]byte(seed))
	return fmt.Sprintf("%x", hash)
}
