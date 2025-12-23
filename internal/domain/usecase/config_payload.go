package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

type ConfigPayload struct {
	Version    string              `json:"version,omitempty"`
	Collect    config.CollectPrefs `json:"collect"`
	Vulns      struct {
		SignaturesURL string `json:"signatures_url"`
	} `json:"vulns"`
	Logs struct {
		WinChannels []string `json:"win_channels"`
		Files       []string `json:"files"`
		BatchLines  int      `json:"batch_lines"`
		MaxBytes    int      `json:"max_bytes"`
		Interval    int      `json:"interval"`
	} `json:"logs"`
	CollectNow *[]string `json:"collect_now,omitempty"`
}

func ApplyConfigPayload(log logger.Logger, store *prefs.Store, commands chan<- string, payload ConfigPayload) (string, bool, error) {
	collect := payload.Collect
	collect.Version = payload.Version
	collect.CVESignaturesURL = payload.Vulns.SignaturesURL
	collect.OSLogWinChList = payload.Logs.WinChannels
	collect.OSLogFilesList = payload.Logs.Files
	if payload.Logs.BatchLines > 0 {
		collect.OSLogBatchLines = payload.Logs.BatchLines
	}
	if payload.Logs.MaxBytes > 0 {
		collect.OSLogMaxBytes = payload.Logs.MaxBytes
	}
	if payload.Logs.Interval > 0 {
		collect.OSLogIntervalSec = payload.Logs.Interval
	}

	collectNow := collect.CollectNow
	if payload.CollectNow != nil {
		collectNow = *payload.CollectNow
	}
	collect.CollectNow = nil

	cur := store.Get()
	if cur.Version == collect.Version && collect.Version != "" && len(collectNow) == 0 {
		return collect.Version, false, nil
	}

	if err := store.Update(collect); err != nil {
		return collect.Version, false, err
	}

	if commands == nil {
		return collect.Version, true, nil
	}

	for _, cmd := range collectNow {
		// non-blocking para não travar o loop
		select {
		case commands <- cmd:
		default:
			log.Info("command channel full, dropping command: " + cmd)
		}
	}
	return collect.Version, true, nil
}
