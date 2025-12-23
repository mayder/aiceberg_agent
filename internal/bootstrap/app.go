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
	"strconv"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/host"
	ps "github.com/shirou/gopsutil/v3/process"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
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
			log.Error("outbox prune failed: " + err.Error() + " target=" + label)
			return
		}
		if removed > 0 {
			log.Info("outbox pruned removed=" + strconv.Itoa(removed) + " target=" + label)
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
			log.Error("ingest config persist failed: " + err.Error())
			return
		}
		if applied {
			log.Info("ingest config applied version=" + version)
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
				log.Error("ingest config persist failed: " + err.Error())
				return
			}
			if applied {
				log.Info("ingest config applied version=" + version)
			}
		}
	}

	flushUC := usecase.NewFlushOutbox(outboxRepo, tx, log, authHeader, onIngestConfig)
	pingUC := usecase.NewPingBackend(cfg, log)
	var counters obsCounters

	var osLogCollectUC *usecase.CollectAndBuffer
	var osLogFlushUC *usecase.FlushOutbox
	var osLogRepo ports.OutboxRepo
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
	tPing = time.NewTicker(cfg.PingInterval)
	tCfgSync = time.NewTicker(cfg.ConfigSyncInterval)
	if osLogCollectUC != nil {
		tOsCollect = time.NewTicker(cfg.OSLogInterval)
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

	log.Info("agent started")

	// coleta de bootstrap imediata (host/inventory estático)
	_ = bootstrapUC.Execute(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
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
			}
		case <-readTick(tOsCollect):
			if osLogCollectUC != nil {
				_ = osLogCollectUC.Execute(ctx)
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
			log.Info("bootstrap skipped (state found)")
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
	log.Info("bootstrap ok")
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
			log.Info("outbox=bolt path=" + path)
			return bs
		} else {
			log.Error("outbox bolt fallback to memory: " + err.Error())
		}
	}
	log.Info("outbox=memory")
	return outbox.NewMemStore()
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
