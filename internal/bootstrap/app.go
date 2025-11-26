package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/host"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/health"
	"github.com/you/aiceberg_agent/internal/platform/collectors/sysmetrics"
)

func Run(cfg config.Config, log logger.Logger) error {
	ctx := context.Background()

	// Adapters mínimos
	store := outbox.NewMemStore()
	outboxRepo := repositories.NewOutboxRepository(store)
	httpTx := transport.NewHTTPJSONClient(cfg)
	prefStore := prefs.NewStore(cfg.PrefsPath)
	_, _ = prefStore.Load()

	if err := bootstrap(ctx, cfg, log); err != nil {
		log.Fatal("bootstrap failed", "err", err)
	}

	// Use cases
	collector := sysmetrics.New(outboxRepo.Len, prefStore.Get)
	collectUC := usecase.NewCollectAndBuffer(collector, outboxRepo, log)
	flushUC := usecase.NewFlushOutbox(outboxRepo, httpTx, log)
	pingUC := usecase.NewPingBackend(cfg, log)
	configSyncUC := usecase.NewConfigSync(cfg, log, prefStore)

	if cfg.HealthPort > 0 {
		go health.Serve(cfg.HealthPort, log)
	}

	tCollect := time.NewTicker(10 * time.Second)
	tFlush := time.NewTicker(15 * time.Second)
	tPing := time.NewTicker(cfg.PingInterval)
	tCfgSync := time.NewTicker(cfg.ConfigSyncInterval)
	defer tCollect.Stop()
	defer tFlush.Stop()
	defer tPing.Stop()
	defer tCfgSync.Stop()

	log.Info("agent started")

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown")
			return nil
		case <-tCollect.C:
			_ = collectUC.Execute(ctx)
		case <-tFlush.C:
			_ = flushUC.Execute(ctx)
		case <-tPing.C:
			_ = pingUC.Execute(ctx)
		case <-tCfgSync.C:
			_ = configSyncUC.Execute(ctx)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIBaseURL+"/v1/agent/bootstrap", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+cfg.Agent.Token)

	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return errors.New("bootstrap rejected: " + resp.Status + " body=" + string(respBody))
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
