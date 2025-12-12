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
	collectUC := usecase.NewCollectAndBuffer(collector, outboxRepo, log, authHeader)
	flushUC := usecase.NewFlushOutbox(outboxRepo, tx, log, authHeader)
	pingUC := usecase.NewPingBackend(cfg, log)
	configSyncUC := usecase.NewConfigSync(cfg, log, prefStore)
	var counters obsCounters

	var osLogCollectUC *usecase.CollectAndBuffer
	var osLogFlushUC *usecase.FlushOutbox
	var osLogRepo ports.OutboxRepo
	if cfg.OSLogEnabled {
		osStore := outbox.NewMemStore()
		osRepo := repositories.NewOutboxRepository(osStore)
		osLogRepo = osRepo
		osCollector := oslogs.New(cfg)
		osLogCollectUC = usecase.NewCollectAndBuffer(osCollector, osRepo, log, authHeader)
		var osTx ports.Transport
		if mode == "relay" {
			osTx = transport.NewHubClient(cfg)
		} else {
			osTx = transport.NewHTTPLogsClient(cfg)
		}
		osLogFlushUC = usecase.NewFlushOutbox(osRepo, osTx, log, authHeader)
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
		go hub.ServeHub(addr, cfg, outboxRepo, log)
	}

	tCollect := time.NewTicker(10 * time.Second)
	tFlush := time.NewTicker(15 * time.Second)
	var tPing *time.Ticker
	var tCfgSync *time.Ticker
	var tOsCollect *time.Ticker
	if mode != "relay" {
		tPing = time.NewTicker(cfg.PingInterval)
		tCfgSync = time.NewTicker(cfg.ConfigSyncInterval)
	}
	if osLogCollectUC != nil {
		tOsCollect = time.NewTicker(cfg.OSLogInterval)
	}
	defer tCollect.Stop()
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

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
			return nil
		case <-tCollect.C:
			start := time.Now()
			if err := collectUC.Execute(ctx); err != nil {
				counters.collectErr.Add(1)
			}
			counters.lastCollectMs.Store(time.Since(start).Milliseconds())
		case <-tFlush.C:
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

func readTick(t *time.Ticker) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
