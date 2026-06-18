package usecase

import (
	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/data/local/prefs"
)

type ControlCommand struct {
	Name          string
	CommandID     string
	CorrelationID string
	Source        string
	CheckIDs      []int
	TimeoutMs     int
	Update        *UpdatePayload
	AutoUpdate    *AutoUpdatePayload
}

type UpdatePayload struct {
	Version string `json:"version,omitempty"`
	URL     string `json:"url,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

func (u *UpdatePayload) Clone() *UpdatePayload {
	if u == nil {
		return nil
	}
	cp := *u
	return &cp
}

type AutoUpdatePayload struct {
	Enabled          *bool   `json:"enabled,omitempty"`
	Dir              *string `json:"dir,omitempty"`
	MaxMB            *int    `json:"max_mb,omitempty"`
	TimeoutSec       *int    `json:"timeout_sec,omitempty"`
	RetryIntervalSec *int    `json:"retry_interval_sec,omitempty"`
	UseAgentAuth     *bool   `json:"use_agent_auth,omitempty"`
	Command          *string `json:"command,omitempty"`
	WorkDir          *string `json:"workdir,omitempty"`
}

func (a *AutoUpdatePayload) Clone() *AutoUpdatePayload {
	if a == nil {
		return nil
	}
	cp := *a
	return &cp
}

func (a *AutoUpdatePayload) HasAnyValue() bool {
	if a == nil {
		return false
	}
	return a.Enabled != nil ||
		a.Dir != nil ||
		a.MaxMB != nil ||
		a.TimeoutSec != nil ||
		a.RetryIntervalSec != nil ||
		a.UseAgentAuth != nil ||
		a.Command != nil ||
		a.WorkDir != nil
}

type ConfigPayload struct {
	Version string              `json:"version,omitempty"`
	Collect config.CollectPrefs `json:"collect"`
	Vulns   struct {
		SignaturesURL string `json:"signatures_url"`
	} `json:"vulns"`
	Logs struct {
		WinChannels  []string `json:"win_channels"`
		Files        []string `json:"files"`
		BatchLines   int      `json:"batch_lines"`
		MaxBytes     int      `json:"max_bytes"`
		Interval     int      `json:"interval"`
		IncludeRegex string   `json:"include_regex,omitempty"`
		ExcludeRegex string   `json:"exclude_regex,omitempty"`
		MinSeverity  string   `json:"min_severity,omitempty"`
	} `json:"logs"`
	CustomMetrics struct {
		Enabled   *bool  `json:"enabled,omitempty"`
		UDPAddr   string `json:"udp_addr,omitempty"`
		HTTPAddr  string `json:"http_addr,omitempty"`
		Interval  int    `json:"interval,omitempty"`
		MaxSeries int    `json:"max_series,omitempty"`
		MaxBytes  int    `json:"max_bytes,omitempty"`
	} `json:"custom_metrics,omitempty"`
	OTLP struct {
		Enabled  *bool  `json:"enabled,omitempty"`
		HTTPAddr string `json:"http_addr,omitempty"`
		Interval int    `json:"interval,omitempty"`
		MaxItems int    `json:"max_items,omitempty"`
		MaxBytes int    `json:"max_bytes,omitempty"`
	} `json:"otlp,omitempty"`
	CollectNow *[]string          `json:"collect_now,omitempty"`
	Update     *UpdatePayload     `json:"update,omitempty"`
	AutoUpdate *AutoUpdatePayload `json:"auto_update,omitempty"`
	Agentless  struct {
		Enabled    *bool `json:"enabled,omitempty"`
		PollSec    int   `json:"poll_interval,omitempty"`
		FlushSec   int   `json:"flush_interval,omitempty"`
		JobsLimit  int   `json:"jobs_limit,omitempty"`
		LockSec    int   `json:"lock_sec,omitempty"`
		FlushBatch int   `json:"flush_batch,omitempty"`
	} `json:"agentless,omitempty"`
}

func ApplyConfigPayload(log logger.Logger, store *prefs.Store, commands chan<- ControlCommand, payload ConfigPayload) (string, bool, error) {
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
	if payload.Logs.IncludeRegex != "" {
		collect.OSLogIncludeRegex = payload.Logs.IncludeRegex
	}
	if payload.Logs.ExcludeRegex != "" {
		collect.OSLogExcludeRegex = payload.Logs.ExcludeRegex
	}
	if payload.Logs.MinSeverity != "" {
		collect.OSLogMinSeverity = payload.Logs.MinSeverity
	}
	if payload.CustomMetrics.Enabled != nil {
		collect.CustomMetricsEnabled = *payload.CustomMetrics.Enabled
	}
	if payload.CustomMetrics.UDPAddr != "" {
		collect.CustomMetricsUDPAddr = payload.CustomMetrics.UDPAddr
	}
	if payload.CustomMetrics.HTTPAddr != "" {
		collect.CustomMetricsHTTPAddr = payload.CustomMetrics.HTTPAddr
	}
	if payload.CustomMetrics.Interval > 0 {
		collect.CustomMetricsIntervalSec = payload.CustomMetrics.Interval
	}
	if payload.CustomMetrics.MaxSeries > 0 {
		collect.CustomMetricsMaxSeries = payload.CustomMetrics.MaxSeries
	}
	if payload.CustomMetrics.MaxBytes > 0 {
		collect.CustomMetricsMaxBytes = payload.CustomMetrics.MaxBytes
	}
	if payload.OTLP.Enabled != nil {
		collect.OTLPEnabled = *payload.OTLP.Enabled
	}
	if payload.OTLP.HTTPAddr != "" {
		collect.OTLPHTTPAddr = payload.OTLP.HTTPAddr
	}
	if payload.OTLP.Interval > 0 {
		collect.OTLPIntervalSec = payload.OTLP.Interval
	}
	if payload.OTLP.MaxItems > 0 {
		collect.OTLPMaxItems = payload.OTLP.MaxItems
	}
	if payload.OTLP.MaxBytes > 0 {
		collect.OTLPMaxBytes = payload.OTLP.MaxBytes
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

	hasUpdate := payload.Update != nil && payload.Update.Version != "" && payload.Update.URL != ""
	// Se o objeto auto_update veio no payload, sempre reaplicamos a política.
	// Isso permite limpar overrides em runtime quando campos voltam para null no backend.
	hasAutoUpdate := payload.AutoUpdate != nil
	cur := store.Get()
	if cur.Version == collect.Version && collect.Version != "" && len(collectNow) == 0 && !hasUpdate && !hasAutoUpdate {
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
		case commands <- ControlCommand{Name: cmd}:
		default:
			log.Info(logger.KV("command channel full, dropping command",
				"command", cmd,
			))
		}
	}

	if hasAutoUpdate {
		select {
		case commands <- ControlCommand{Name: "self_update_policy", AutoUpdate: payload.AutoUpdate.Clone()}:
		default:
			log.Info(logger.KV("command channel full, dropping command",
				"command", "self_update_policy",
			))
		}
	}
	if hasUpdate {
		select {
		case commands <- ControlCommand{Name: "self_update", Update: payload.Update.Clone()}:
		default:
			log.Info(logger.KV("command channel full, dropping command",
				"command", "self_update",
				"version", payload.Update.Version,
			))
		}
	}
	return collect.Version, true, nil
}
