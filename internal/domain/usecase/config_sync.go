package usecase

import (
	"context"
	"encoding/json"
	"net/http"
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
		uc.recordFailure(err)
		uc.log.Error(logger.KV("config sync failed",
			"route", "/v1/agent/config",
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

	var payload ConfigPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
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
		return err
	}
	if applied {
		uc.log.Info(logger.KV("config sync ok",
			"version", version,
		))
	}
	uc.backoff.Reset()
	return nil
}

func (uc *ConfigSync) recordFailure(err error) {
	delay, kind := uc.backoff.Failure(err)
	if kind != retry.ErrorKindTransient {
		delay = uc.backoff.Cooldown(retry.DefaultMaxBackoff)
		uc.log.Error(logger.KV("config sync permanent failure",
			"route", "/v1/agent/config",
			"err_kind", kind,
			"retry_after_ms", delay.Milliseconds(),
			"err", err,
		))
		return
	}
	uc.log.Info(logger.KV("config sync backoff scheduled",
		"route", "/v1/agent/config",
		"err_kind", kind,
		"retry_after_ms", delay.Milliseconds(),
	))
}
