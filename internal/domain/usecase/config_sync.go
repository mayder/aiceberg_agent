package usecase

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

// ConfigSync faz pull periódico das preferências de coleta do backend e persiste localmente.
type ConfigSync struct {
	cfg      config.Config
	log      logger.Logger
	store    *prefs.Store
	cl       *http.Client
	commands chan<- string
}

func NewConfigSync(cfg config.Config, log logger.Logger, store *prefs.Store, commands chan<- string) *ConfigSync {
	return &ConfigSync{
		cfg:      cfg,
		log:      log,
		store:    store,
		cl:       httpx.NewClient(cfg, 8*time.Second),
		commands: commands,
	}
}

func (uc *ConfigSync) Execute(ctx context.Context) error {
	url := uc.cfg.APIEndpoint("/v1/agent/config")
	if uc.cfg.AgentMode == "relay" && uc.cfg.HubURL != "" {
		url = uc.cfg.HubURL + "/v1/agent/config"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	httpx.SetAuth(req, uc.cfg)

	resp, err := uc.cl.Do(req)
	if err != nil {
		uc.log.Error("config sync failed: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode >= 300 {
		return &httpStatusErr{code: resp.StatusCode}
	}

	var payload ConfigPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	version, applied, err := ApplyConfigPayload(uc.log, uc.store, uc.commands, payload)
	if err != nil {
		uc.log.Error("config persist failed: " + err.Error())
		return err
	}
	if applied {
		uc.log.Info("config sync ok version=" + version)
	}
	return nil
}
