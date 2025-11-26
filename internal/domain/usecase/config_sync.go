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
	cfg   config.Config
	log   logger.Logger
	store *prefs.Store
	cl    *http.Client
}

func NewConfigSync(cfg config.Config, log logger.Logger, store *prefs.Store) *ConfigSync {
	return &ConfigSync{
		cfg:   cfg,
		log:   log,
		store: store,
		cl:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (uc *ConfigSync) Execute(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uc.cfg.APIEndpoint("/v1/agent/config"), nil)
	if err != nil {
		return err
	}
	httpx.SetAuth(req, uc.cfg)

	resp, err := uc.cl.Do(req)
	if err != nil {
		uc.log.Error("config sync: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode >= 300 {
		return &httpStatusErr{code: resp.StatusCode}
	}

	var payload struct {
		Version string              `json:"version,omitempty"`
		Collect config.CollectPrefs `json:"collect"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	// Preencher versão no struct interno.
	payload.Collect.Version = payload.Version

	cur := uc.store.Get()
	if cur.Version == payload.Collect.Version && payload.Collect.Version != "" {
		return nil
	}

	if err := uc.store.Update(payload.Collect); err != nil {
		uc.log.Error("config persist: " + err.Error())
		return err
	}
	uc.log.Info("config sync ok version=" + payload.Collect.Version)
	return nil
}
