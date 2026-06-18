package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type AgentCfg struct {
	LogLevel string `json:"log_level"`
	Token    string `json:"token"`
}

type LocalCheckConfig struct {
	ID             string            `json:"id,omitempty"`
	Kind           string            `json:"kind,omitempty"`
	Version        string            `json:"version,omitempty"`
	IntervalSec    int               `json:"interval,omitempty"`
	TimeoutMs      int               `json:"timeout_ms,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Target         string            `json:"target,omitempty"`
	CredentialsRef string            `json:"credentials_ref,omitempty"`
	Config         map[string]string `json:"config,omitempty"`
	Enabled        bool              `json:"enabled,omitempty"`
}

type Config struct {
	Agent                              AgentCfg
	APIBaseURL                         string
	APIKey                             string
	HTTPGzip                           bool
	HTTPIdempotency                    bool
	IngestTimeout                      time.Duration
	OutboxFlushBatch                   int
	OutboxFlushInterval                time.Duration
	TLSInsecureSkip                    bool
	TLSInsecureAllowProd               bool
	RemoteConfigSignatureSecret        string
	RemoteConfigSignatureRequired      bool
	RemoteConfigAllowUnsignedSensitive bool
	OutboxPath                         string
	OutboxMaxMB                        int
	OutboxMaxPerAgent                  int
	HealthPort                         int
	PingInterval                       time.Duration
	ConfigSyncInterval                 time.Duration
	ChannelHeartbeatInterval           time.Duration
	PrefsPath                          string
	AgentMode                          string
	AgentModeOverridePath              string
	HubURL                             string
	HubToken                           string
	HubListenAddr                      string
	SkipBootstrap                      bool
	OSLogEnabled                       bool
	OSLogFiles                         []string
	OSLogCursorPath                    string
	OSLogBatchLines                    int
	OSLogMaxBytes                      int
	OSLogInterval                      time.Duration
	OSLogWinChannels                   []string
	OSLogEnrich                        bool
	OSLogDetections                    bool
	OSLogDiag                          bool
	OSLogIncludeRegex                  string
	OSLogExcludeRegex                  string
	OSLogMinSeverity                   string
	OSLogUDPAddr                       string
	OSLogTCPAddr                       string
	CustomMetricsEnabled               bool
	CustomMetricsUDPAddr               string
	CustomMetricsHTTPAddr              string
	CustomMetricsUDSPath               string
	CustomMetricsInterval              time.Duration
	CustomMetricsMaxSeries             int
	CustomMetricsMaxBytes              int
	OTLPEnabled                        bool
	OTLPHTTPAddr                       string
	OTLPInterval                       time.Duration
	OTLPMaxItems                       int
	OTLPMaxBytes                       int
	APMTraceSampleRate                 float64
	APMTraceSlowThresholdMs            int
	APMTracePreserveErrors             bool
	ContainerEnabled                   bool
	ContainerRuntime                   string
	ContainerDockerSocket              string
	ContainerContainerdSocket          string
	ContainerContainerdNamespace       string
	ContainerCtrPath                   string
	ContainerInterval                  time.Duration
	ContainerMaxItems                  int
	ContainerIncludeRegex              string
	ContainerExcludeRegex              string
	ContainerLogsEnabled               bool
	ContainerLogsCursorPath            string
	ContainerLogsMaxLines              int
	ContainerLogsMaxBytes              int
	KubernetesEnabled                  bool
	KubernetesAPIURL                   string
	KubernetesTokenPath                string
	KubernetesCAPath                   string
	KubernetesNodeName                 string
	KubernetesNamespace                string
	KubernetesInterval                 time.Duration
	KubernetesMaxItems                 int
	KubernetesMaxEvents                int
	KubernetesLogsEnabled              bool
	KubernetesLogsCursorPath           string
	KubernetesLogsMaxLines             int
	KubernetesLogsMaxBytes             int
	KubernetesLogsIncludeRegex         string
	KubernetesLogsExcludeRegex         string
	LocalChecksEnabled                 bool
	LocalChecksInterval                time.Duration
	LocalChecksMaxChecks               int
	LocalChecksMaxBytes                int
	LocalChecks                        []LocalCheckConfig
	LocalCheckManifestDirs             []string
	AgentlessEnabled                   bool
	AgentlessPollInterval              time.Duration
	AgentlessFlushInterval             time.Duration
	SelfHealPollInterval               time.Duration
	AgentlessOutboxPath                string
	AgentlessOutboxMaxMB               int
	AgentlessJobsLimit                 int
	AgentlessLockSec                   int
	AgentlessFlushBatch                int
	AgentlessDebug                     bool
	AutoUpdateEnabled                  bool
	AutoUpdateDir                      string
	AutoUpdateCommand                  string
	AutoUpdateWorkDir                  string
	AutoUpdateTimeout                  time.Duration
	AutoUpdateRetryInterval            time.Duration
	AutoUpdateMaxMB                    int
	AutoUpdateUseAgentAuth             bool
	AgentClientID                      int
	AgentID                            int
	AgentInstallationID                string
	AgentIdentitySecret                string
}

func parseLocalChecks(raw string) []LocalCheckConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var checks []LocalCheckConfig
	if err := json.Unmarshal([]byte(raw), &checks); err == nil {
		return checks
	}
	return nil
}

func parseCSV(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

type CollectPrefs struct {
	Version                    string             `json:"version,omitempty"`
	Paused                     bool               `json:"paused,omitempty"`
	CPU                        bool               `json:"cpu"`
	Memory                     bool               `json:"memory"`
	Disk                       bool               `json:"disk"`
	Network                    bool               `json:"network"`
	NetActive                  bool               `json:"net_active"`
	Host                       bool               `json:"host"`
	Sensors                    bool               `json:"sensors"`
	Power                      bool               `json:"power"`
	Sanity                     bool               `json:"sanity"`
	GPU                        bool               `json:"gpu"`
	Services                   bool               `json:"services"`
	TimeSync                   bool               `json:"time_sync"`
	Logs                       bool               `json:"logs"`
	Updates                    bool               `json:"updates"`
	Agent                      bool               `json:"agent"`
	Processes                  bool               `json:"processes"`
	Vulns                      bool               `json:"vulns"`
	Inventory                  bool               `json:"inventory"`
	OSLogEnrich                bool               `json:"oslog_enrich"`
	OSLogDetections            bool               `json:"oslog_detections"`
	OSLogDiag                  bool               `json:"oslog_diag"`
	OSLogWinChannels           bool               `json:"oslog_win_channels"`
	OSLogFiles                 bool               `json:"oslog_files"`
	CollectNow                 []string           `json:"collect_now,omitempty"`
	CVESignaturesURL           string             `json:"cve_signatures_url,omitempty"`
	OSLogWinChList             []string           `json:"oslog_win_channels_list,omitempty"`
	OSLogFilesList             []string           `json:"oslog_files_list,omitempty"`
	OSLogBatchLines            int                `json:"oslog_batch_lines,omitempty"`
	OSLogMaxBytes              int                `json:"oslog_max_bytes,omitempty"`
	OSLogIntervalSec           int                `json:"oslog_interval,omitempty"`
	OSLogIncludeRegex          string             `json:"oslog_include_regex,omitempty"`
	OSLogExcludeRegex          string             `json:"oslog_exclude_regex,omitempty"`
	OSLogMinSeverity           string             `json:"oslog_min_severity,omitempty"`
	OSLogUDPAddr               string             `json:"oslog_udp_addr,omitempty"`
	OSLogTCPAddr               string             `json:"oslog_tcp_addr,omitempty"`
	CustomMetricsEnabled       bool               `json:"custom_metrics_enabled,omitempty"`
	CustomMetricsUDPAddr       string             `json:"custom_metrics_udp_addr,omitempty"`
	CustomMetricsHTTPAddr      string             `json:"custom_metrics_http_addr,omitempty"`
	CustomMetricsUDSPath       string             `json:"custom_metrics_uds_path,omitempty"`
	CustomMetricsIntervalSec   int                `json:"custom_metrics_interval,omitempty"`
	CustomMetricsMaxSeries     int                `json:"custom_metrics_max_series,omitempty"`
	CustomMetricsMaxBytes      int                `json:"custom_metrics_max_bytes,omitempty"`
	OTLPEnabled                bool               `json:"otlp_enabled,omitempty"`
	OTLPHTTPAddr               string             `json:"otlp_http_addr,omitempty"`
	OTLPIntervalSec            int                `json:"otlp_interval,omitempty"`
	OTLPMaxItems               int                `json:"otlp_max_items,omitempty"`
	OTLPMaxBytes               int                `json:"otlp_max_bytes,omitempty"`
	APMTraceSampleRate         float64            `json:"apm_trace_sample_rate,omitempty"`
	APMTraceSlowThresholdMs    int                `json:"apm_trace_slow_threshold_ms,omitempty"`
	APMTracePreserveErrors     bool               `json:"apm_trace_preserve_errors,omitempty"`
	ContainerEnabled           bool               `json:"container_enabled,omitempty"`
	ContainerRuntime           string             `json:"container_runtime,omitempty"`
	ContainerDockerSocket      string             `json:"container_docker_socket,omitempty"`
	ContainerContainerdSocket  string             `json:"container_containerd_socket,omitempty"`
	ContainerContainerdNS      string             `json:"container_containerd_namespace,omitempty"`
	ContainerCtrPath           string             `json:"container_ctr_path,omitempty"`
	ContainerIntervalSec       int                `json:"container_interval,omitempty"`
	ContainerMaxItems          int                `json:"container_max_items,omitempty"`
	ContainerIncludeRegex      string             `json:"container_include_regex,omitempty"`
	ContainerExcludeRegex      string             `json:"container_exclude_regex,omitempty"`
	ContainerLogsEnabled       bool               `json:"container_logs_enabled,omitempty"`
	ContainerLogsCursorPath    string             `json:"container_logs_cursor_path,omitempty"`
	ContainerLogsMaxLines      int                `json:"container_logs_max_lines,omitempty"`
	ContainerLogsMaxBytes      int                `json:"container_logs_max_bytes,omitempty"`
	KubernetesEnabled          bool               `json:"kubernetes_enabled,omitempty"`
	KubernetesAPIURL           string             `json:"kubernetes_api_url,omitempty"`
	KubernetesTokenPath        string             `json:"kubernetes_token_path,omitempty"`
	KubernetesCAPath           string             `json:"kubernetes_ca_path,omitempty"`
	KubernetesNodeName         string             `json:"kubernetes_node_name,omitempty"`
	KubernetesNamespace        string             `json:"kubernetes_namespace,omitempty"`
	KubernetesIntervalSec      int                `json:"kubernetes_interval,omitempty"`
	KubernetesMaxItems         int                `json:"kubernetes_max_items,omitempty"`
	KubernetesMaxEvents        int                `json:"kubernetes_max_events,omitempty"`
	KubernetesLogsEnabled      bool               `json:"kubernetes_logs_enabled,omitempty"`
	KubernetesLogsCursorPath   string             `json:"kubernetes_logs_cursor_path,omitempty"`
	KubernetesLogsMaxLines     int                `json:"kubernetes_logs_max_lines,omitempty"`
	KubernetesLogsMaxBytes     int                `json:"kubernetes_logs_max_bytes,omitempty"`
	KubernetesLogsIncludeRegex string             `json:"kubernetes_logs_include_regex,omitempty"`
	KubernetesLogsExcludeRegex string             `json:"kubernetes_logs_exclude_regex,omitempty"`
	LocalChecksEnabled         bool               `json:"local_checks_enabled,omitempty"`
	LocalChecksIntervalSec     int                `json:"local_checks_interval,omitempty"`
	LocalChecksMaxChecks       int                `json:"local_checks_max_checks,omitempty"`
	LocalChecksMaxBytes        int                `json:"local_checks_max_bytes,omitempty"`
	LocalChecks                []LocalCheckConfig `json:"local_checks,omitempty"`
	LocalCheckManifestDirs     []string           `json:"local_check_manifest_dirs,omitempty"`
	AgentlessEnabled           bool               `json:"agentless_enabled,omitempty"`
	AgentlessPollSec           int                `json:"agentless_poll_interval,omitempty"`
	AgentlessFlushSec          int                `json:"agentless_flush_interval,omitempty"`
	AgentlessJobsLimit         int                `json:"agentless_jobs_limit,omitempty"`
	AgentlessLockSec           int                `json:"agentless_lock_sec,omitempty"`
	AgentlessFlushBatch        int                `json:"agentless_flush_batch,omitempty"`
	NetworkPassiveMode         string             `json:"network_passive_mode,omitempty"`
	NetworkCaptureWindowSec    int                `json:"network_capture_window_sec,omitempty"`
	NetworkCaptureSampleSec    int                `json:"network_capture_sample_sec,omitempty"`
	NetworkCaptureMaxFlows     int                `json:"network_capture_max_flows,omitempty"`
	NetworkCaptureMaxPeers     int                `json:"network_capture_max_peers,omitempty"`
	NetworkCaptureMaxListeners int                `json:"network_capture_max_listeners,omitempty"`
	NetworkCaptureTimeoutMs    int                `json:"network_capture_timeout_ms,omitempty"`
	NetworkAdvancedEnabled     bool               `json:"network_advanced_enabled,omitempty"`
	USMEnabled                 bool               `json:"usm_enabled,omitempty"`
	WorkloadSecurityEnabled    bool               `json:"workload_security_enabled,omitempty"`
	NetworkPCAPEnabled         bool               `json:"network_pcap_enabled,omitempty"`
	NetworkPCAPIface           string             `json:"network_pcap_iface,omitempty"`
	NetworkPCAPDurationSec     int                `json:"network_pcap_duration_sec,omitempty"`
	NetworkPCAPMaxPackets      int                `json:"network_pcap_max_packets,omitempty"`
	NetworkExternalSources     []string           `json:"network_external_sources,omitempty"`
	NetworkExternalMaxRecords  int                `json:"network_external_max_records,omitempty"`
}

func Load(configPath string) (Config, error) {
	// Carrega envs de arquivo (flag -config ou AGENT_ENV_FILE) sem sobrescrever o que já está no ambiente.
	envFile := getenv("AGENT_ENV_FILE", "")
	if configPath != "" {
		loadEnvFile(configPath)
	} else if envFile != "" {
		loadEnvFile(envFile)
	}

	port := 0
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	pingInterval := time.Duration(intEnv("PING_INTERVAL", 5)) * time.Second
	cfgSyncInterval := time.Duration(intEnv("CONFIG_SYNC_INTERVAL", 30)) * time.Second
	channelHeartbeatInterval := time.Duration(intEnv("CHANNEL_HEARTBEAT_INTERVAL", 30)) * time.Second
	ingestTimeout := time.Duration(intEnv("INGEST_TIMEOUT_SEC", 10)) * time.Second
	outboxFlushInterval := time.Duration(intEnv("OUTBOX_FLUSH_INTERVAL", 15)) * time.Second
	agentlessPoll := time.Duration(intEnv("AGENTLESS_POLL_INTERVAL", 30)) * time.Second
	agentlessFlush := time.Duration(intEnv("AGENTLESS_FLUSH_INTERVAL", 15)) * time.Second
	selfHealPoll := time.Duration(intEnv("SELFHEAL_POLL_INTERVAL", 30)) * time.Second
	autoUpdateTimeout := time.Duration(intEnv("AUTO_UPDATE_TIMEOUT", 300)) * time.Second
	autoUpdateRetry := time.Duration(intEnv("AUTO_UPDATE_RETRY_INTERVAL", 1800)) * time.Second
	cfg := Config{
		Agent:                              AgentCfg{LogLevel: getenv("LOG_LEVEL", "info"), Token: loadToken()},
		APIBaseURL:                         getenv("API_BASE_URL", "https://api.aiceberg.com.br"),
		APIKey:                             getenv("API_KEY", ""),
		HTTPGzip:                           strings.ToLower(getenv("HTTP_GZIP", "")) == "true",
		HTTPIdempotency:                    strings.ToLower(getenv("HTTP_IDEMPOTENCY", "true")) == "true",
		OutboxFlushBatch:                   intEnv("OUTBOX_FLUSH_BATCH", 50),
		TLSInsecureSkip:                    strings.ToLower(getenv("TLS_INSECURE_SKIP_VERIFY", "")) == "true",
		TLSInsecureAllowProd:               strings.ToLower(getenv("TLS_INSECURE_ALLOW_PROD", "")) == "true",
		RemoteConfigSignatureSecret:        getenv("REMOTE_CONFIG_SIGNATURE_SECRET", ""),
		RemoteConfigSignatureRequired:      strings.ToLower(getenv("REMOTE_CONFIG_SIGNATURE_REQUIRED", "")) == "true",
		RemoteConfigAllowUnsignedSensitive: strings.ToLower(getenv("REMOTE_CONFIG_ALLOW_UNSIGNED_SENSITIVE", "")) == "true",
		OutboxPath:                         getenv("OUTBOX_PATH", "./data/outbox.db"),
		OutboxMaxMB:                        intEnv("OUTBOX_MAX_MB", 200),
		OutboxMaxPerAgent:                  intEnv("OUTBOX_MAX_PER_AGENT", 0),
		HealthPort:                         port,
		PrefsPath:                          getenv("PREFS_PATH", "./data/collect_prefs.json"),
		AgentMode:                          strings.ToLower(getenv("AGENT_MODE", "direct")),
		AgentModeOverridePath:              getenv("AGENT_MODE_OVERRIDE_PATH", ""),
		HubURL:                             getenv("HUB_URL", ""),
		HubToken:                           getenv("HUB_TOKEN", ""),
		HubListenAddr:                      getenv("HUB_LISTEN_ADDR", ""),
		SkipBootstrap:                      strings.ToLower(getenv("SKIP_BOOTSTRAP", "")) == "true",
		OSLogEnabled:                       strings.ToLower(getenv("OSLOG_ENABLED", "")) == "true",
		OSLogFiles:                         splitCsv(getenv("OSLOG_FILES", "")),
		OSLogCursorPath:                    getenv("OSLOG_CURSOR_PATH", "./data/oslogs.cursor"),
		OSLogBatchLines:                    intEnv("OSLOG_BATCH_LINES", 200),
		OSLogMaxBytes:                      intEnv("OSLOG_MAX_BYTES", 256*1024),
		OSLogInterval:                      time.Duration(intEnv("OSLOG_INTERVAL", 15)) * time.Second,
		OSLogWinChannels:                   splitCsv(getenv("OSLOG_WIN_CHANNELS", "")),
		OSLogEnrich:                        strings.ToLower(getenv("OSLOG_ENRICH", "")) == "true",
		OSLogDetections:                    strings.ToLower(getenv("OSLOG_DETECTIONS", "")) == "true",
		OSLogDiag:                          strings.ToLower(getenv("OSLOG_DIAG", "")) == "true",
		OSLogIncludeRegex:                  getenv("OSLOG_INCLUDE_REGEX", ""),
		OSLogExcludeRegex:                  getenv("OSLOG_EXCLUDE_REGEX", ""),
		OSLogMinSeverity:                   getenv("OSLOG_MIN_SEVERITY", ""),
		OSLogUDPAddr:                       getenv("OSLOG_UDP_ADDR", ""),
		OSLogTCPAddr:                       getenv("OSLOG_TCP_ADDR", ""),
		CustomMetricsEnabled:               strings.ToLower(getenv("CUSTOM_METRICS_ENABLED", "")) == "true",
		CustomMetricsUDPAddr:               getenv("CUSTOM_METRICS_UDP_ADDR", "127.0.0.1:8125"),
		CustomMetricsHTTPAddr:              getenv("CUSTOM_METRICS_HTTP_ADDR", "127.0.0.1:8126"),
		CustomMetricsUDSPath:               getenv("CUSTOM_METRICS_UDS_PATH", ""),
		CustomMetricsInterval:              time.Duration(intEnv("CUSTOM_METRICS_INTERVAL", 10)) * time.Second,
		CustomMetricsMaxSeries:             intEnv("CUSTOM_METRICS_MAX_SERIES", 1000),
		CustomMetricsMaxBytes:              intEnv("CUSTOM_METRICS_MAX_BYTES", 65536),
		OTLPEnabled:                        strings.ToLower(getenv("OTLP_ENABLED", "")) == "true",
		OTLPHTTPAddr:                       getenv("OTLP_HTTP_ADDR", "127.0.0.1:4318"),
		OTLPInterval:                       time.Duration(intEnv("OTLP_INTERVAL", 10)) * time.Second,
		OTLPMaxItems:                       intEnv("OTLP_MAX_ITEMS", 1000),
		OTLPMaxBytes:                       intEnv("OTLP_MAX_BYTES", 1024*1024),
		APMTraceSampleRate:                 floatEnv("APM_TRACE_SAMPLE_RATE", 1),
		APMTraceSlowThresholdMs:            intEnv("APM_TRACE_SLOW_THRESHOLD_MS", 1000),
		APMTracePreserveErrors:             strings.ToLower(getenv("APM_TRACE_PRESERVE_ERRORS", "true")) != "false",
		ContainerEnabled:                   strings.ToLower(getenv("CONTAINER_ENABLED", "")) == "true",
		ContainerRuntime:                   getenv("CONTAINER_RUNTIME", "auto"),
		ContainerDockerSocket:              getenv("CONTAINER_DOCKER_SOCKET", "/var/run/docker.sock"),
		ContainerContainerdSocket:          getenv("CONTAINER_CONTAINERD_SOCKET", "/run/containerd/containerd.sock"),
		ContainerContainerdNamespace:       getenv("CONTAINER_CONTAINERD_NAMESPACE", "k8s.io"),
		ContainerCtrPath:                   getenv("CONTAINER_CTR_PATH", "ctr"),
		ContainerInterval:                  time.Duration(intEnv("CONTAINER_INTERVAL", 30)) * time.Second,
		ContainerMaxItems:                  intEnv("CONTAINER_MAX_ITEMS", 200),
		ContainerIncludeRegex:              getenv("CONTAINER_INCLUDE_REGEX", ""),
		ContainerExcludeRegex:              getenv("CONTAINER_EXCLUDE_REGEX", ""),
		ContainerLogsEnabled:               strings.ToLower(getenv("CONTAINER_LOGS_ENABLED", "")) == "true",
		ContainerLogsCursorPath:            getenv("CONTAINER_LOGS_CURSOR_PATH", "./data/container_logs.cursor"),
		ContainerLogsMaxLines:              intEnv("CONTAINER_LOGS_MAX_LINES", 200),
		ContainerLogsMaxBytes:              intEnv("CONTAINER_LOGS_MAX_BYTES", 256*1024),
		KubernetesEnabled:                  strings.ToLower(getenv("KUBERNETES_ENABLED", "")) == "true",
		KubernetesAPIURL:                   getenv("KUBERNETES_API_URL", "https://kubernetes.default.svc"),
		KubernetesTokenPath:                getenv("KUBERNETES_TOKEN_PATH", "/var/run/secrets/kubernetes.io/serviceaccount/token"),
		KubernetesCAPath:                   getenv("KUBERNETES_CA_PATH", "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"),
		KubernetesNodeName:                 getenv("KUBERNETES_NODE_NAME", ""),
		KubernetesNamespace:                getenv("KUBERNETES_NAMESPACE", ""),
		KubernetesInterval:                 time.Duration(intEnv("KUBERNETES_INTERVAL", 30)) * time.Second,
		KubernetesMaxItems:                 intEnv("KUBERNETES_MAX_ITEMS", 500),
		KubernetesMaxEvents:                intEnv("KUBERNETES_MAX_EVENTS", 100),
		KubernetesLogsEnabled:              strings.ToLower(getenv("KUBERNETES_LOGS_ENABLED", "")) == "true",
		KubernetesLogsCursorPath:           getenv("KUBERNETES_LOGS_CURSOR_PATH", "./data/kubernetes_logs.cursor"),
		KubernetesLogsMaxLines:             intEnv("KUBERNETES_LOGS_MAX_LINES", 200),
		KubernetesLogsMaxBytes:             intEnv("KUBERNETES_LOGS_MAX_BYTES", 256*1024),
		KubernetesLogsIncludeRegex:         getenv("KUBERNETES_LOGS_INCLUDE_REGEX", ""),
		KubernetesLogsExcludeRegex:         getenv("KUBERNETES_LOGS_EXCLUDE_REGEX", ""),
		LocalChecksEnabled:                 strings.ToLower(getenv("LOCAL_CHECKS_ENABLED", "")) == "true",
		LocalChecksInterval:                time.Duration(intEnv("LOCAL_CHECKS_INTERVAL", 30)) * time.Second,
		LocalChecksMaxChecks:               intEnv("LOCAL_CHECKS_MAX_CHECKS", 100),
		LocalChecksMaxBytes:                intEnv("LOCAL_CHECKS_MAX_BYTES", 1024*1024),
		LocalChecks:                        parseLocalChecks(getenv("LOCAL_CHECKS_JSON", "")),
		LocalCheckManifestDirs:             parseCSV(getenv("LOCAL_CHECKS_MANIFEST_DIRS", "./integrations/localchecks/manifests")),
		AgentlessEnabled:                   strings.ToLower(getenv("AGENTLESS_ENABLED", "true")) == "true",
		AgentlessOutboxPath:                getenv("AGENTLESS_OUTBOX_PATH", "./data/agentless_outbox.db"),
		AgentlessOutboxMaxMB:               intEnv("AGENTLESS_OUTBOX_MAX_MB", 50),
		AgentlessJobsLimit:                 intEnv("AGENTLESS_JOBS_LIMIT", 50),
		AgentlessLockSec:                   intEnv("AGENTLESS_LOCK_SEC", 60),
		AgentlessFlushBatch:                intEnv("AGENTLESS_FLUSH_BATCH", 100),
		AgentlessDebug:                     strings.ToLower(getenv("AGENTLESS_DEBUG", "")) == "true",
		AutoUpdateEnabled:                  strings.ToLower(getenv("AUTO_UPDATE_ENABLED", "")) == "true",
		AutoUpdateDir:                      getenv("AUTO_UPDATE_DIR", "./data/updates"),
		AutoUpdateCommand:                  getenv("AUTO_UPDATE_COMMAND", ""),
		AutoUpdateWorkDir:                  getenv("AUTO_UPDATE_WORKDIR", ""),
		AutoUpdateMaxMB:                    intEnv("AUTO_UPDATE_MAX_MB", 300),
		AutoUpdateUseAgentAuth:             strings.ToLower(getenv("AUTO_UPDATE_USE_AGENT_AUTH", "")) == "true",
		AgentClientID:                      intEnv("AGENT_CLIENT_ID", 0),
		AgentID:                            intEnv("AGENT_ID", 0),
		AgentInstallationID:                getenv("AGENT_INSTALLATION_ID", ""),
		AgentIdentitySecret:                getenv("AGENT_IDENTITY_SECRET", ""),
		PingInterval: func() time.Duration {
			if pingInterval <= 0 {
				return 5 * time.Second
			}
			return pingInterval
		}(),
		ConfigSyncInterval: func() time.Duration {
			if cfgSyncInterval <= 0 {
				return 30 * time.Second
			}
			return cfgSyncInterval
		}(),
		ChannelHeartbeatInterval: func() time.Duration {
			if channelHeartbeatInterval <= 0 {
				return 30 * time.Second
			}
			return channelHeartbeatInterval
		}(),
		IngestTimeout: func() time.Duration {
			if ingestTimeout <= 0 {
				return 10 * time.Second
			}
			return ingestTimeout
		}(),
		OutboxFlushInterval: func() time.Duration {
			if outboxFlushInterval <= 0 {
				return 15 * time.Second
			}
			return outboxFlushInterval
		}(),
		AgentlessPollInterval: func() time.Duration {
			if agentlessPoll <= 0 {
				return 30 * time.Second
			}
			return agentlessPoll
		}(),
		AgentlessFlushInterval: func() time.Duration {
			if agentlessFlush <= 0 {
				return 15 * time.Second
			}
			return agentlessFlush
		}(),
		SelfHealPollInterval: func() time.Duration {
			if selfHealPoll <= 0 {
				return 30 * time.Second
			}
			return selfHealPoll
		}(),
		AutoUpdateTimeout: func() time.Duration {
			if autoUpdateTimeout <= 0 {
				return 5 * time.Minute
			}
			return autoUpdateTimeout
		}(),
		AutoUpdateRetryInterval: func() time.Duration {
			if autoUpdateRetry <= 0 {
				return 30 * time.Minute
			}
			return autoUpdateRetry
		}(),
	}
	if cfg.AgentModeOverridePath == "" {
		cfg.AgentModeOverridePath = filepath.Join(filepath.Dir(cfg.PrefsPath), "agent_mode.override")
	}
	if override := loadAgentModeOverride(cfg.AgentModeOverridePath); override != "" {
		cfg.AgentMode = override
	}
	if cfg.TLSInsecureSkip && !cfg.TLSInsecureAllowProd && strings.Contains(strings.ToLower(cfg.APIBaseURL), "api.aiceberg.com.br") {
		return cfg, fmt.Errorf("TLS_INSECURE_SKIP_VERIFY bloqueado para API de producao")
	}
	if cfg.Agent.Token == "" {
		return cfg, fmt.Errorf("AGENT_TOKEN obrigatório")
	}
	return cfg, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loadToken() string {
	if v := os.Getenv("AGENT_TOKEN"); v != "" {
		return v
	}
	path := getenv("AGENT_TOKEN_PATH", "./data/agent.token")
	if b, err := os.ReadFile(path); err == nil {
		return string(b)
	}
	return ""
}

func (c Config) APIEndpoint(segment string) string {
	base := strings.TrimRight(c.APIBaseURL, "/")
	if segment == "" {
		return base
	}
	if !strings.HasPrefix(segment, "/") {
		segment = "/" + segment
	}
	return base + segment
}

func (c Config) Mode() string {
	switch strings.ToLower(c.AgentMode) {
	case "hub", "relay", "direct":
		return strings.ToLower(c.AgentMode)
	default:
		return "direct"
	}
}

func loadAgentModeOverride(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "direct", "direto":
		return "direct"
	case "hub":
		return "hub"
	case "relay":
		return "relay"
	default:
		return ""
	}
}

func splitCsv(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func floatEnv(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			return n
		}
	}
	return def
}

// loadEnvFile lê um arquivo (env-style, JSON ou YAML simples) e injeta variáveis de ambiente
// somente quando ainda não existem no ambiente atual.
func loadEnvFile(path string) {
	if path == "" {
		return
	}
	vars := parseConfigFile(path)
	for k, v := range vars {
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}

func parseConfigFile(path string) map[string]string {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return map[string]string{}
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return parseJSONMap(raw)
	case ".yaml", ".yml":
		if m := parseYAMLMap(raw); len(m) > 0 {
			return m
		}
	}
	if m := parseEnvLines(raw); len(m) > 0 {
		return m
	}
	// fallback: tentar JSON por último mesmo se extensão diferente
	return parseJSONMap(raw)
}

func parseJSONMap(raw string) map[string]string {
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return map[string]string{}
	}
	return flattenStringMap(data, "")
}

func parseYAMLMap(raw string) map[string]string {
	var data map[string]any
	if err := yaml.Unmarshal([]byte(raw), &data); err != nil {
		return map[string]string{}
	}
	return flattenStringMap(data, "")
}

// parseEnvLines lê formato KEY=VALUE ou KEY: VALUE (ignora linhas vazias/# comentário).
func parseEnvLines(raw string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		sep := strings.IndexAny(ln, "=:")
		if sep <= 0 {
			continue
		}
		k := strings.TrimSpace(ln[:sep])
		v := strings.TrimSpace(ln[sep+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

// flattenStringMap achata mapas simples convertendo valores em string.
func flattenStringMap(in map[string]any, prefix string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case fmt.Stringer:
			out[key] = val.String()
		case map[string]any:
			for nk, nv := range flattenStringMap(val, key) {
				out[nk] = nv
			}
		default:
			out[key] = fmt.Sprint(val)
		}
	}
	return out
}
