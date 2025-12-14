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
		cl:    httpx.NewClient(cfg, 8*time.Second),
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

	var payload struct {
		Version string              `json:"version,omitempty"`
		Collect config.CollectPrefs `json:"collect"`
		Vulns   struct {
			SignaturesURL string `json:"signatures_url"`
		} `json:"vulns"`
		Logs struct {
			WinChannels []string `json:"win_channels"`
			Files       []string `json:"files"`
			BatchLines  int      `json:"batch_lines"`
			MaxBytes    int      `json:"max_bytes"`
			Interval    int      `json:"interval"`
		} `json:"logs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	// Preencher versão no struct interno.
	payload.Collect.Version = payload.Version
	payload.Collect.CVESignaturesURL = payload.Vulns.SignaturesURL
	payload.Collect.OSLogWinChList = payload.Logs.WinChannels
	payload.Collect.OSLogFilesList = payload.Logs.Files
	if payload.Logs.BatchLines > 0 {
		payload.Collect.OSLogBatchLines = payload.Logs.BatchLines
	}
	if payload.Logs.MaxBytes > 0 {
		payload.Collect.OSLogMaxBytes = payload.Logs.MaxBytes
	}
	if payload.Logs.Interval > 0 {
		payload.Collect.OSLogIntervalSec = payload.Logs.Interval
	}

	cur := uc.store.Get()
	if cur.Version == payload.Collect.Version && payload.Collect.Version != "" {
		return nil
	}

	if err := uc.store.Update(payload.Collect); err != nil {
		uc.log.Error("config persist failed: " + err.Error())
		return err
	}
	uc.log.Info("config sync ok version=" + payload.Collect.Version)
	return nil
}
