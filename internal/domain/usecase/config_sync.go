package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/data/local/configstate"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

// ConfigSync faz pull periódico das preferências de coleta do backend e persiste localmente.
type ConfigSync struct {
	cfg              config.Config
	log              logger.Logger
	store            *prefs.Store
	cl               *http.Client
	commands         chan<- ControlCommand
	backoff          *retry.Backoff
	forceFullSync    atomic.Bool
	checkpointAfter  time.Time
	mu               sync.Mutex
	now              func() time.Time
	failureState     configfailure.State
	failureStore     *configstate.Store
	durable          bool
	persistAfter     time.Time
	applyDigest      string
	applyUntil       time.Time
	applyBackoff     *retry.Backoff
	reportBackoff    *retry.Backoff
	attemptedReports map[string]bool
}

func NewConfigSync(cfg config.Config, log logger.Logger, store *prefs.Store, commands chan<- ControlCommand) *ConfigSync {
	uc := &ConfigSync{
		cfg:              cfg,
		log:              log,
		store:            store,
		cl:               httpx.NewClient(cfg, 8*time.Second),
		commands:         commands,
		backoff:          retry.NewBackoff(),
		attemptedReports: make(map[string]bool),
		now:              time.Now, applyBackoff: retry.NewBackoff(), reportBackoff: retry.NewBackoff(),
	}
	uc.restoreConfigDiagnostics()
	return uc
}

func (uc *ConfigSync) Execute(ctx context.Context) error {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.failureState.Expire(uc.now().UnixMilli()) {
		uc.persistConfigDiagnostics()
	}
	uc.flushConfigDiagnostics(ctx)
	defer uc.flushConfigDiagnostics(ctx)
	if until, ok := uc.backoff.Active(); ok {
		uc.log.Info(logger.KV("config sync backoff active",
			"route", "/v1/agent/config",
			"retry_after_ms", time.Until(until).Milliseconds(),
		))
		return nil
	}
	payload, configHash, unchanged, err := uc.fetchConfigPayload(ctx)
	if err != nil {
		return err
	}
	if unchanged {
		uc.backoff.Reset()
		return nil
	}

	version, applied, err := uc.applyConfig(payload, configHash)
	if err != nil {
		return err
	}
	uc.forceFullSync.Store(false)
	if version != "" && len(uc.failureState.Pending) == 0 {
		if reportErr := uc.reportConfigApply(ctx, version, configHash, "applied", configApplyMessage(applied)); reportErr != nil {
			uc.RequestFullSync()
			uc.log.Info(logger.KV("config report deferred", "version", version, "status", "applied"))
		}
	}
	uc.backoff.Reset()
	return nil
}

func (uc *ConfigSync) recordFailure(err error) retry.ErrorKind {
	delay, kind := uc.backoff.Failure(err)
	if kind != retry.ErrorKindTransient {
		delay = uc.backoff.Cooldown(retry.DefaultMaxBackoff)
		uc.log.Error(logger.KV("config sync permanent failure",
			"route", "/v1/agent/config",
			"err_kind", kind,
			"retry_after_ms", delay.Milliseconds(),
			"err", err,
		))
		return kind
	}
	uc.log.Info(logger.KV("config sync backoff scheduled",
		"route", "/v1/agent/config",
		"err_kind", kind,
		"retry_after_ms", delay.Milliseconds(),
	))
	return kind
}

func (uc *ConfigSync) reportConfigApply(ctx context.Context, version string, configHash string, status string, message string, diagnostic ...configfailure.Report) error {
	status = strings.TrimSpace(status)
	if status == "" {
		return nil
	}
	body := map[string]any{
		"status":         status,
		"config_version": strings.TrimSpace(version),
		"config_hash":    strings.TrimSpace(configHash),
		"message":        strings.TrimSpace(message),
		"ts_unix_ms":     time.Now().UnixMilli(),
	}
	if len(diagnostic) > 0 {
		body["diagnostic"] = diagnostic[0]
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	uc.log.Info(logger.KV("config report prepared", "version", version, "bytes", len(raw)))

	url := uc.cfg.APIEndpoint("/v1/agent/config-report")
	if uc.cfg.AgentMode == "relay" && strings.TrimSpace(uc.cfg.HubURL) != "" {
		url = strings.TrimRight(strings.TrimSpace(uc.cfg.HubURL), "/") + "/v1/agent/config-report"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpx.SetAuth(req, uc.cfg)
	if identityHeader := uc.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}
	started := time.Now()
	resp, err := uc.cl.Do(req)
	metrics.ConfigReportSent(int64(len(raw)), time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("config-report status=%d: %w", resp.StatusCode, &httpStatusErr{code: resp.StatusCode})
}

func configApplyMessage(applied bool) string {
	if applied {
		return "configuration applied locally"
	}
	return "configuration already applied locally"
}

func hashPayload(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (uc *ConfigSync) requestConfig(ctx context.Context) (*http.Response, error) {
	url := uc.cfg.APIEndpoint("/v1/agent/config")
	if uc.cfg.AgentMode == "relay" && uc.cfg.HubURL != "" {
		url = uc.cfg.HubURL + "/v1/agent/config"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if currentVersion := strings.TrimSpace(uc.store.Get().Version); currentVersion != "" && !uc.forceFullSync.Load() {
		query := req.URL.Query()
		query.Set("version", currentVersion)
		req.URL.RawQuery = query.Encode()
	}
	httpx.SetAuth(req, uc.cfg)
	if identityHeader := uc.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}

	return uc.cl.Do(req)
}

func (uc *ConfigSync) fetchConfigPayload(ctx context.Context) (ConfigPayload, string, bool, error) {
	resp, err := uc.requestConfig(ctx)
	if err != nil {
		uc.recordFailure(err)
		return ConfigPayload{}, "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return ConfigPayload{}, "", true, nil
	}
	if resp.StatusCode >= 300 {
		err = &httpStatusErr{code: resp.StatusCode}
		uc.recordFailure(err)
		return ConfigPayload{}, "", false, err
	}
	const maxPayload = 1 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPayload+1))
	if err == nil && len(body) > maxPayload {
		err = fmt.Errorf("configuration payload exceeds size limit")
	}
	var payload ConfigPayload
	if err == nil {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil {
		uc.recordFailure(err)
		return ConfigPayload{}, "", false, err
	}
	return payload, hashPayload(body), false, nil
}
