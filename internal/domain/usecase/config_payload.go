package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

type ConfigPayload struct {
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
	CollectNow *[]string `json:"collect_now,omitempty"`
	Agentless  struct {
		Enabled    *bool `json:"enabled,omitempty"`
		PollSec    int   `json:"poll_interval,omitempty"`
		FlushSec   int   `json:"flush_interval,omitempty"`
		JobsLimit  int   `json:"jobs_limit,omitempty"`
		LockSec    int   `json:"lock_sec,omitempty"`
		FlushBatch int   `json:"flush_batch,omitempty"`
	} `json:"agentless,omitempty"`
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
	if payload.Agentless.Enabled != nil {
		collect.AgentlessEnabled = *payload.Agentless.Enabled
	}
	if payload.Agentless.PollSec > 0 {
		collect.AgentlessPollSec = payload.Agentless.PollSec
	}
	if payload.Agentless.FlushSec > 0 {
		collect.AgentlessFlushSec = payload.Agentless.FlushSec
	}
	if payload.Agentless.JobsLimit > 0 {
		collect.AgentlessJobsLimit = payload.Agentless.JobsLimit
	}
	if payload.Agentless.LockSec > 0 {
		collect.AgentlessLockSec = payload.Agentless.LockSec
	}
	if payload.Agentless.FlushBatch > 0 {
		collect.AgentlessFlushBatch = payload.Agentless.FlushBatch
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
			log.Info(logger.KV("command channel full, dropping command",
				"command", cmd,
			))
		}
	}
	return collect.Version, true, nil
}
