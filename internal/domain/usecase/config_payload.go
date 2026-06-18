package usecase

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
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
	TokenRotation *TokenRotationPayload
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

type TokenRotationPayload struct {
	NewToken          string `json:"new_token,omitempty"`
	PreviousExpiresAt string `json:"previous_expires_at,omitempty"`
	Reason            string `json:"reason,omitempty"`
}

func (t *TokenRotationPayload) Clone() *TokenRotationPayload {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}

type PayloadSignature struct {
	Algorithm string `json:"algorithm,omitempty"`
	KeyID     string `json:"key_id,omitempty"`
	Value     string `json:"value,omitempty"`
	SignedAt  string `json:"signed_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Scope     string `json:"scope,omitempty"`
}

type ConfigSecurityOptions struct {
	SignatureSecret        string
	SignatureRequired      bool
	AllowUnsignedSensitive bool
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
	Version   string              `json:"version,omitempty"`
	Signature PayloadSignature    `json:"signature,omitempty"`
	Collect   config.CollectPrefs `json:"collect"`
	Vulns     struct {
		SignaturesURL string `json:"signatures_url"`
	} `json:"vulns"`
	Logs struct {
		WinChannels  []string                    `json:"win_channels"`
		WinProviders []string                    `json:"win_providers,omitempty"`
		WinEventIDs  []string                    `json:"win_event_ids,omitempty"`
		Files        []string                    `json:"files"`
		BatchLines   int                         `json:"batch_lines"`
		MaxBytes     int                         `json:"max_bytes"`
		Interval     int                         `json:"interval"`
		IncludeRegex string                      `json:"include_regex,omitempty"`
		ExcludeRegex string                      `json:"exclude_regex,omitempty"`
		MinSeverity  string                      `json:"min_severity,omitempty"`
		UDPAddr      string                      `json:"udp_addr,omitempty"`
		TCPAddr      string                      `json:"tcp_addr,omitempty"`
		Journald     *bool                       `json:"journald_enabled,omitempty"`
		JournalUnits []string                    `json:"journald_units,omitempty"`
		JournalPrio  []string                    `json:"journald_priorities,omitempty"`
		Processors   []config.LogProcessorConfig `json:"processors,omitempty"`
	} `json:"logs"`
	CustomMetrics struct {
		Enabled   *bool  `json:"enabled,omitempty"`
		UDPAddr   string `json:"udp_addr,omitempty"`
		HTTPAddr  string `json:"http_addr,omitempty"`
		UDSPath   string `json:"uds_path,omitempty"`
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
	APM struct {
		TraceSampleRate      *float64 `json:"trace_sample_rate,omitempty"`
		TraceSlowThresholdMs int      `json:"trace_slow_threshold_ms,omitempty"`
		TracePreserveErrors  *bool    `json:"trace_preserve_errors,omitempty"`
	} `json:"apm,omitempty"`
	Containers struct {
		Enabled             *bool  `json:"enabled,omitempty"`
		Runtime             string `json:"runtime,omitempty"`
		DockerSocket        string `json:"docker_socket,omitempty"`
		ContainerdSocket    string `json:"containerd_socket,omitempty"`
		ContainerdNamespace string `json:"containerd_namespace,omitempty"`
		CtrPath             string `json:"ctr_path,omitempty"`
		Interval            int    `json:"interval,omitempty"`
		MaxItems            int    `json:"max_items,omitempty"`
		IncludeRegex        string `json:"include_regex,omitempty"`
		ExcludeRegex        string `json:"exclude_regex,omitempty"`
		LogsEnabled         *bool  `json:"logs_enabled,omitempty"`
		LogsMaxLines        int    `json:"logs_max_lines,omitempty"`
		LogsMaxBytes        int    `json:"logs_max_bytes,omitempty"`
	} `json:"containers,omitempty"`
	Kubernetes struct {
		Enabled          *bool  `json:"enabled,omitempty"`
		APIURL           string `json:"api_url,omitempty"`
		TokenPath        string `json:"token_path,omitempty"`
		CAPath           string `json:"ca_path,omitempty"`
		NodeName         string `json:"node_name,omitempty"`
		Namespace        string `json:"namespace,omitempty"`
		Interval         int    `json:"interval,omitempty"`
		MaxItems         int    `json:"max_items,omitempty"`
		MaxEvents        int    `json:"max_events,omitempty"`
		LogsEnabled      *bool  `json:"logs_enabled,omitempty"`
		LogsCursorPath   string `json:"logs_cursor_path,omitempty"`
		LogsMaxLines     int    `json:"logs_max_lines,omitempty"`
		LogsMaxBytes     int    `json:"logs_max_bytes,omitempty"`
		LogsIncludeRegex string `json:"logs_include_regex,omitempty"`
		LogsExcludeRegex string `json:"logs_exclude_regex,omitempty"`
	} `json:"kubernetes,omitempty"`
	LocalChecks struct {
		Enabled      *bool                     `json:"enabled,omitempty"`
		Interval     int                       `json:"interval,omitempty"`
		MaxChecks    int                       `json:"max_checks,omitempty"`
		MaxBytes     int                       `json:"max_bytes,omitempty"`
		ManifestDirs []string                  `json:"manifest_dirs,omitempty"`
		Checks       []config.LocalCheckConfig `json:"checks,omitempty"`
	} `json:"local_checks,omitempty"`
	CollectNow    *[]string             `json:"collect_now,omitempty"`
	Update        *UpdatePayload        `json:"update,omitempty"`
	AutoUpdate    *AutoUpdatePayload    `json:"auto_update,omitempty"`
	TokenRotation *TokenRotationPayload `json:"token_rotation,omitempty"`
	Agentless     struct {
		Enabled    *bool `json:"enabled,omitempty"`
		PollSec    int   `json:"poll_interval,omitempty"`
		FlushSec   int   `json:"flush_interval,omitempty"`
		JobsLimit  int   `json:"jobs_limit,omitempty"`
		LockSec    int   `json:"lock_sec,omitempty"`
		FlushBatch int   `json:"flush_batch,omitempty"`
	} `json:"agentless,omitempty"`
}

func ApplyConfigPayload(log logger.Logger, store *prefs.Store, commands chan<- ControlCommand, payload ConfigPayload) (string, bool, error) {
	return ApplyConfigPayloadWithSecurity(log, store, commands, payload, ConfigSecurityOptions{})
}

func ApplyConfigPayloadWithSecurity(log logger.Logger, store *prefs.Store, commands chan<- ControlCommand, payload ConfigPayload, security ConfigSecurityOptions) (string, bool, error) {
	if err := ValidateConfigPayloadSecurity(payload, security); err != nil {
		return payload.Version, false, err
	}
	if err := validateUpdatePolicy(payload.Update); err != nil {
		return payload.Version, false, err
	}
	collect := payload.Collect
	collect.Version = payload.Version
	collect.CVESignaturesURL = payload.Vulns.SignaturesURL
	collect.OSLogWinChList = payload.Logs.WinChannels
	collect.OSLogWinProviders = payload.Logs.WinProviders
	collect.OSLogWinEventIDs = payload.Logs.WinEventIDs
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
	if payload.Logs.UDPAddr != "" {
		collect.OSLogUDPAddr = payload.Logs.UDPAddr
	}
	if payload.Logs.TCPAddr != "" {
		collect.OSLogTCPAddr = payload.Logs.TCPAddr
	}
	if payload.Logs.Journald != nil {
		collect.OSLogJournaldEnabled = *payload.Logs.Journald
	}
	if len(payload.Logs.JournalUnits) > 0 {
		collect.OSLogJournaldUnits = payload.Logs.JournalUnits
	}
	if len(payload.Logs.JournalPrio) > 0 {
		collect.OSLogJournaldPriorities = payload.Logs.JournalPrio
	}
	if len(payload.Logs.Processors) > 0 {
		collect.OSLogProcessors = payload.Logs.Processors
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
	if payload.CustomMetrics.UDSPath != "" {
		collect.CustomMetricsUDSPath = payload.CustomMetrics.UDSPath
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
	if payload.APM.TraceSampleRate != nil {
		collect.APMTraceSampleRate = *payload.APM.TraceSampleRate
	}
	if payload.APM.TraceSlowThresholdMs > 0 {
		collect.APMTraceSlowThresholdMs = payload.APM.TraceSlowThresholdMs
	}
	if payload.APM.TracePreserveErrors != nil {
		collect.APMTracePreserveErrors = *payload.APM.TracePreserveErrors
	}
	if payload.Containers.Enabled != nil {
		collect.ContainerEnabled = *payload.Containers.Enabled
	}
	if payload.Containers.Runtime != "" {
		collect.ContainerRuntime = payload.Containers.Runtime
	}
	if payload.Containers.DockerSocket != "" {
		collect.ContainerDockerSocket = payload.Containers.DockerSocket
	}
	if payload.Containers.ContainerdSocket != "" {
		collect.ContainerContainerdSocket = payload.Containers.ContainerdSocket
	}
	if payload.Containers.ContainerdNamespace != "" {
		collect.ContainerContainerdNS = payload.Containers.ContainerdNamespace
	}
	if payload.Containers.CtrPath != "" {
		collect.ContainerCtrPath = payload.Containers.CtrPath
	}
	if payload.Containers.Interval > 0 {
		collect.ContainerIntervalSec = payload.Containers.Interval
	}
	if payload.Containers.MaxItems > 0 {
		collect.ContainerMaxItems = payload.Containers.MaxItems
	}
	if payload.Containers.IncludeRegex != "" {
		collect.ContainerIncludeRegex = payload.Containers.IncludeRegex
	}
	if payload.Containers.ExcludeRegex != "" {
		collect.ContainerExcludeRegex = payload.Containers.ExcludeRegex
	}
	if payload.Containers.LogsEnabled != nil {
		collect.ContainerLogsEnabled = *payload.Containers.LogsEnabled
	}
	if payload.Containers.LogsMaxLines > 0 {
		collect.ContainerLogsMaxLines = payload.Containers.LogsMaxLines
	}
	if payload.Containers.LogsMaxBytes > 0 {
		collect.ContainerLogsMaxBytes = payload.Containers.LogsMaxBytes
	}
	if payload.Kubernetes.Enabled != nil {
		collect.KubernetesEnabled = *payload.Kubernetes.Enabled
	}
	if payload.Kubernetes.APIURL != "" {
		collect.KubernetesAPIURL = payload.Kubernetes.APIURL
	}
	if payload.Kubernetes.TokenPath != "" {
		collect.KubernetesTokenPath = payload.Kubernetes.TokenPath
	}
	if payload.Kubernetes.CAPath != "" {
		collect.KubernetesCAPath = payload.Kubernetes.CAPath
	}
	if payload.Kubernetes.NodeName != "" {
		collect.KubernetesNodeName = payload.Kubernetes.NodeName
	}
	if payload.Kubernetes.Namespace != "" {
		collect.KubernetesNamespace = payload.Kubernetes.Namespace
	}
	if payload.Kubernetes.Interval > 0 {
		collect.KubernetesIntervalSec = payload.Kubernetes.Interval
	}
	if payload.Kubernetes.MaxItems > 0 {
		collect.KubernetesMaxItems = payload.Kubernetes.MaxItems
	}
	if payload.Kubernetes.MaxEvents > 0 {
		collect.KubernetesMaxEvents = payload.Kubernetes.MaxEvents
	}
	if payload.Kubernetes.LogsEnabled != nil {
		collect.KubernetesLogsEnabled = *payload.Kubernetes.LogsEnabled
	}
	if payload.Kubernetes.LogsCursorPath != "" {
		collect.KubernetesLogsCursorPath = payload.Kubernetes.LogsCursorPath
	}
	if payload.Kubernetes.LogsMaxLines > 0 {
		collect.KubernetesLogsMaxLines = payload.Kubernetes.LogsMaxLines
	}
	if payload.Kubernetes.LogsMaxBytes > 0 {
		collect.KubernetesLogsMaxBytes = payload.Kubernetes.LogsMaxBytes
	}
	if payload.Kubernetes.LogsIncludeRegex != "" {
		collect.KubernetesLogsIncludeRegex = payload.Kubernetes.LogsIncludeRegex
	}
	if payload.Kubernetes.LogsExcludeRegex != "" {
		collect.KubernetesLogsExcludeRegex = payload.Kubernetes.LogsExcludeRegex
	}
	if payload.LocalChecks.Enabled != nil {
		collect.LocalChecksEnabled = *payload.LocalChecks.Enabled
	}
	if payload.LocalChecks.Interval > 0 {
		collect.LocalChecksIntervalSec = payload.LocalChecks.Interval
	}
	if payload.LocalChecks.MaxChecks > 0 {
		collect.LocalChecksMaxChecks = payload.LocalChecks.MaxChecks
	}
	if payload.LocalChecks.MaxBytes > 0 {
		collect.LocalChecksMaxBytes = payload.LocalChecks.MaxBytes
	}
	if payload.LocalChecks.Checks != nil {
		collect.LocalChecks = payload.LocalChecks.Checks
	}
	if payload.LocalChecks.ManifestDirs != nil {
		collect.LocalCheckManifestDirs = payload.LocalChecks.ManifestDirs
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
	hasTokenRotation := payload.TokenRotation != nil && strings.TrimSpace(payload.TokenRotation.NewToken) != ""
	// Se o objeto auto_update veio no payload, sempre reaplicamos a política.
	// Isso permite limpar overrides em runtime quando campos voltam para null no backend.
	hasAutoUpdate := payload.AutoUpdate != nil
	cur := store.Get()
	if cur.Version == collect.Version && collect.Version != "" && len(collectNow) == 0 && !hasUpdate && !hasAutoUpdate && !hasTokenRotation {
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
	if hasTokenRotation {
		select {
		case commands <- ControlCommand{Name: "rotate_agent_token", TokenRotation: payload.TokenRotation.Clone()}:
		default:
			log.Info(logger.KV("command channel full, dropping command",
				"command", "rotate_agent_token",
			))
		}
	}
	return collect.Version, true, nil
}

func ValidateConfigPayloadSecurity(payload ConfigPayload, opts ConfigSecurityOptions) error {
	sensitive := payloadHasSensitiveConfig(payload)
	if !opts.SignatureRequired && strings.TrimSpace(opts.SignatureSecret) == "" {
		return nil
	}
	if !opts.SignatureRequired && (!sensitive || opts.AllowUnsignedSensitive) {
		return nil
	}
	if strings.TrimSpace(opts.SignatureSecret) == "" {
		if opts.SignatureRequired || sensitive {
			return errors.New("remote config signature secret missing")
		}
		return nil
	}
	if strings.TrimSpace(payload.Signature.Value) == "" {
		if opts.SignatureRequired || sensitive {
			return errors.New("remote config signature missing")
		}
		return nil
	}
	if payload.Signature.Algorithm != "" && !strings.EqualFold(payload.Signature.Algorithm, "hmac-sha256") {
		return fmt.Errorf("remote config signature algorithm unsupported: %s", payload.Signature.Algorithm)
	}
	if payload.Signature.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, payload.Signature.ExpiresAt)
		if err != nil {
			return fmt.Errorf("remote config signature expires_at invalid: %w", err)
		}
		if time.Now().After(expiresAt) {
			return errors.New("remote config signature expired")
		}
	}
	expected, err := SignConfigPayload(payload, opts.SignatureSecret)
	if err != nil {
		return err
	}
	got := strings.TrimSpace(payload.Signature.Value)
	if !hmac.Equal([]byte(strings.ToLower(got)), []byte(strings.ToLower(expected))) {
		return errors.New("remote config signature invalid")
	}
	return nil
}

func SignConfigPayload(payload ConfigPayload, secret string) (string, error) {
	payload.Signature.Value = ""
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func payloadHasSensitiveConfig(payload ConfigPayload) bool {
	return payload.Update != nil ||
		payload.AutoUpdate != nil ||
		payload.TokenRotation != nil ||
		payload.CollectNow != nil ||
		payload.LocalChecks.Checks != nil ||
		payload.LocalChecks.ManifestDirs != nil ||
		strings.TrimSpace(payload.Containers.Runtime) != "" ||
		strings.TrimSpace(payload.Containers.DockerSocket) != "" ||
		strings.TrimSpace(payload.Containers.ContainerdSocket) != "" ||
		strings.TrimSpace(payload.Containers.ContainerdNamespace) != "" ||
		strings.TrimSpace(payload.Containers.CtrPath) != "" ||
		strings.TrimSpace(payload.Kubernetes.TokenPath) != ""
}

func validateUpdatePolicy(payload *UpdatePayload) error {
	if payload == nil || payload.Version == "" {
		return nil
	}
	cmp := compareVersionStrings(payload.Version, version.Version)
	if cmp < 0 && !payload.Force {
		return fmt.Errorf("update downgrade blocked: %s -> %s", version.Version, payload.Version)
	}
	return nil
}

func compareVersionStrings(a, b string) int {
	as := versionParts(a)
	bs := versionParts(b)
	max := len(as)
	if len(bs) > max {
		max = len(bs)
	}
	for i := 0; i < max; i++ {
		av, bv := 0, 0
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}

func versionParts(value string) []int {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		n := 0
		for _, r := range field {
			if r < '0' || r > '9' {
				break
			}
			n = n*10 + int(r-'0')
		}
		out = append(out, n)
	}
	return out
}
