package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/health"
	"github.com/you/aiceberg_agent/internal/interfaces/hub"
	"github.com/you/aiceberg_agent/internal/platform/collectors/oslogs"
	"github.com/you/aiceberg_agent/internal/platform/collectors/sysmetrics"
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

	var tx ports.Transport
	if mode == "relay" {
		tx = transport.NewHubClient(cfg)
	} else {
		tx = transport.NewHTTPJSONClient(cfg)
	}

	proc := processHandle()

	collector := sysmetrics.New(outboxRepo.Len, prefStore.Get)

	metricsEndpoint := "/v1/ingest/metrics"
	healthEndpoint := "/v1/ingest/health"
	inventoryEndpoint := "/v1/ingest/inventory"
	bootstrapEndpoint := "/v1/ingest/bootstrap"

	metricsUC := usecase.NewCollectAndBuffer(newFilteredCollector(collector, "sysmetrics", metricsEndpoint, 10*time.Second, metricsKeys()), outboxRepo, log, authHeader, metricsEndpoint)
	healthUC := usecase.NewCollectAndBuffer(newFilteredCollector(collector, "sysmetrics_health", healthEndpoint, 10*time.Minute, healthKeys()), outboxRepo, log, authHeader, healthEndpoint)
	inventoryUC := usecase.NewCollectAndBuffer(newFilteredCollector(collector, "sysmetrics_inventory", inventoryEndpoint, 8*time.Hour, inventoryKeys()), outboxRepo, log, authHeader, inventoryEndpoint)
	bootstrapUC := usecase.NewCollectAndBuffer(newFilteredCollector(collector, "sysmetrics_bootstrap", bootstrapEndpoint, 24*time.Hour, bootstrapKeys()), outboxRepo, log, authHeader, bootstrapEndpoint)

	commandChan := make(chan string, 10)
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

	flushUC := usecase.NewFlushOutbox(outboxRepo, tx, log, authHeader, onIngestConfig)
	pingUC := usecase.NewPingBackend(cfg, log)
	var counters obsCounters

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
	osLogCollectUC = usecase.NewCollectAndBuffer(osCollector, osRepo, log, authHeader, "/v1/logs/raw")
	var osTx ports.Transport
	if mode == "relay" {
		osTx = transport.NewHubClient(cfg)
	} else {
		osTx = transport.NewHTTPLogsClient(cfg)
	}
	osLogFlushUC = usecase.NewFlushOutbox(osRepo, osTx, log, authHeader, onIngestConfig)
	pruneStore(osStore, "oslogs")

	agentlessSettings := func() usecase.AgentlessSettings {
		p := prefStore.Get()
		enabled := cfg.AgentlessEnabled
		if p.AgentlessEnabled {
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

	if mode == "hub" && cfg.AgentlessEnabled {
		agentlessStore := selectAgentlessOutbox(cfg, log)
		agentlessRepo = repositories.NewAgentlessOutboxRepository(agentlessStore)
		agentlessClient := agentlessremote.NewAgentlessHubClient(cfg)
		targetsStore := selectAgentlessTargetsStore(log)
		agentlessUC = usecase.NewAgentlessHub(cfg, log, agentlessClient, agentlessRepo, agentlessSettings, targetsStore)
		agentlessLastPoll = time.Now().Add(-cfg.AgentlessPollInterval)
		agentlessLastFlush = time.Now().Add(-cfg.AgentlessFlushInterval)
	}

	if cfg.HealthPort > 0 {
		go health.Serve(cfg.HealthPort, log, func() health.Snapshot {
			items, bytes := outboxRepo.Len()
			if osLogRepo != nil {
				oi, ob := osLogRepo.Len()
				items += oi
				bytes += ob
			}
			procRSS, procCPU := procStats(proc)
			return health.Snapshot{
				Status:         "ok",
				QueueItems:     items,
				QueueBytes:     bytes,
				FlushOK:        counters.flushOK.Load(),
				FlushErr:       counters.flushErr.Load(),
				CollectErr:     counters.collectErr.Load(),
				InvalidEnv:     metrics.InvalidEnvelopesTotal(),
				AgentlessJobs:  metrics.AgentlessJobsTotal(),
				UptimeSec:      int64(time.Since(startedAt).Seconds()),
				ProcRSS:        procRSS,
				ProcCPU:        procCPU,
				Goroutines:     runtime.NumGoroutine(),
				LastCollectMs:  counters.lastCollectMs.Load(),
				LastFlushMs:    counters.lastFlushMs.Load(),
				LastFlushBatch: counters.lastFlushBatch.Load(),
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
	tFlush := time.NewTicker(15 * time.Second)
	var tPing *time.Ticker
	var tCfgSync *time.Ticker
	var tOsCollect *time.Ticker
	var tAgentlessTick *time.Ticker
	tPing = time.NewTicker(cfg.PingInterval)
	tCfgSync = time.NewTicker(cfg.ConfigSyncInterval)
	if osLogCollectUC != nil {
		tOsCollect = time.NewTicker(cfg.OSLogInterval)
	}
	if agentlessUC != nil {
		tAgentlessTick = time.NewTicker(5 * time.Second)
	}
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
	if tAgentlessTick != nil {
		defer tAgentlessTick.Stop()
	}

	log.Info(logger.KV("agent started",
		"mode", mode,
	))

	// coleta de bootstrap imediata (host/inventory estático)
	_ = bootstrapUC.Execute(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info(logger.KV("shutdown"))
			return nil
		case <-tMetrics.C:
			start := time.Now()
			if err := metricsUC.Execute(ctx); err != nil {
				counters.collectErr.Add(1)
			}
			counters.lastCollectMs.Store(time.Since(start).Milliseconds())
		case <-tHealth.C:
			_ = healthUC.Execute(ctx)
		case <-tInventory.C:
			_ = inventoryUC.Execute(ctx)
		case <-tFlush.C:
			pruneStore(store, "main")
			pruneStore(osStore, "oslogs")
			start := time.Now()
			if n, err := flushUC.Execute(ctx); err != nil {
				counters.flushErr.Add(1)
			} else {
				counters.flushOK.Add(1)
				counters.lastFlushBatch.Store(int64(n))
			}
			counters.lastFlushMs.Store(time.Since(start).Milliseconds())
			if osLogFlushUC != nil {
				start := time.Now()
				if n, err := osLogFlushUC.Execute(ctx); err != nil {
					counters.flushErr.Add(1)
				} else {
					counters.flushOK.Add(1)
					counters.lastFlushBatch.Store(int64(n))
				}
				counters.lastFlushMs.Store(time.Since(start).Milliseconds())
			}
		case <-readTick(tPing):
			_ = pingUC.Execute(ctx)
		case <-readTick(tCfgSync):
			_ = configSyncUC.Execute(ctx)
		case cmd := <-commandChan:
			switch cmd {
			case "inventory":
				_ = inventoryUC.Execute(ctx)
			case "health":
				_ = healthUC.Execute(ctx)
			case "bootstrap":
				_ = bootstrapUC.Execute(ctx)
			case "agentless":
				if agentlessUC != nil {
					if err := agentlessUC.SyncTargets(ctx); err != nil {
						log.Error(logger.KV("agentless targets sync failed",
							"err", err,
						))
					}
					agentlessUC.CollectNow(ctx)
				}
			}
		case <-readTick(tOsCollect):
			if osLogCollectUC != nil {
				_ = osLogCollectUC.Execute(ctx)
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
							}
							_ = agentlessUC.PollAndRun(ctx)
						}
						if runFlush {
							_ = agentlessUC.Flush(ctx)
						}
					}(shouldPoll, shouldFlush)
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
