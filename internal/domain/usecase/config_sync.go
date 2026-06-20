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
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

// ConfigSync faz pull periódico das preferências de coleta do backend e persiste localmente.
type ConfigSync struct {
	cfg      config.Config
	log      logger.Logger
	store    *prefs.Store
	cl       *http.Client
	commands chan<- ControlCommand
	backoff  *retry.Backoff
}

func NewConfigSync(cfg config.Config, log logger.Logger, store *prefs.Store, commands chan<- ControlCommand) *ConfigSync {
	return &ConfigSync{
		cfg:      cfg,
		log:      log,
		store:    store,
		cl:       httpx.NewClient(cfg, 8*time.Second),
		commands: commands,
		backoff:  retry.NewBackoff(),
	}
}

func (uc *ConfigSync) Execute(ctx context.Context) error {
	if until, ok := uc.backoff.Active(); ok {
		uc.log.Info(logger.KV("config sync backoff active",
			"route", "/v1/agent/config",
			"retry_after_ms", time.Until(until).Milliseconds(),
		))
		return nil
	}
	url := uc.cfg.APIEndpoint("/v1/agent/config")
	if uc.cfg.AgentMode == "relay" && uc.cfg.HubURL != "" {
		url = uc.cfg.HubURL + "/v1/agent/config"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	httpx.SetAuth(req, uc.cfg)
	if identityHeader := uc.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}

	resp, err := uc.cl.Do(req)
	if err != nil {
		kind := uc.recordFailure(err)
		if kind == retry.ErrorKindTransient {
			uc.log.Info(logger.KV("config sync transient failure",
				"route", "/v1/agent/config",
				"err_kind", kind,
				"err", err,
			))
			return err
		}
		uc.log.Error(logger.KV("config sync failed",
			"route", "/v1/agent/config",
			"err_kind", kind,
			"err", err,
		))
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		uc.backoff.Reset()
		return nil
	}
	if resp.StatusCode >= 300 {
		err := &httpStatusErr{code: resp.StatusCode}
		uc.recordFailure(err)
		return err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		uc.recordFailure(err)
		return err
	}
	configHash := hashPayload(body)
	var payload ConfigPayload
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&payload); err != nil {
		uc.recordFailure(err)
		return err
	}
	version, applied, err := ApplyConfigPayloadWithSecurity(uc.log, uc.store, uc.commands, payload, ConfigSecurityOptions{
		SignatureSecret:        uc.cfg.RemoteConfigSignatureSecret,
		SignatureRequired:      uc.cfg.RemoteConfigSignatureRequired,
		AllowUnsignedSensitive: uc.cfg.RemoteConfigAllowUnsignedSensitive,
	})
	if err != nil {
		uc.log.Error(logger.KV("config persist failed",
			"version", version,
			"err", err,
		))
		if reportErr := uc.reportConfigApply(ctx, version, configHash, "apply_failed", err.Error()); reportErr != nil {
			uc.log.Error(logger.KV("config report failed",
				"version", version,
				"status", "apply_failed",
				"err", reportErr,
			))
		}
		return err
	}
	if reportErr := uc.reportConfigApply(ctx, version, configHash, "applied", configApplyMessage(applied)); reportErr != nil {
		uc.log.Error(logger.KV("config report failed",
			"version", version,
			"status", "applied",
			"err", reportErr,
		))
	}
	if applied {
		uc.log.Info(logger.KV("config sync ok",
			"version", version,
		))
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

func (uc *ConfigSync) reportConfigApply(ctx context.Context, version string, configHash string, status string, message string) error {
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
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

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
	resp, err := uc.cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("config-report status=%d", resp.StatusCode)
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
