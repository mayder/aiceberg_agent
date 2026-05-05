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

type Config struct {
	Agent                    AgentCfg
	APIBaseURL               string
	APIKey                   string
	HTTPGzip                 bool
	HTTPIdempotency          bool
	IngestTimeout            time.Duration
	OutboxFlushBatch         int
	OutboxFlushInterval      time.Duration
	TLSInsecureSkip          bool
	OutboxPath               string
	OutboxMaxMB              int
	OutboxMaxPerAgent        int
	HealthPort               int
	PingInterval             time.Duration
	ConfigSyncInterval       time.Duration
	ChannelHeartbeatInterval time.Duration
	PrefsPath                string
	AgentMode                string
	AgentModeOverridePath    string
	HubURL                   string
	HubToken                 string
	HubListenAddr            string
	SkipBootstrap            bool
	OSLogEnabled             bool
	OSLogFiles               []string
	OSLogCursorPath          string
	OSLogBatchLines          int
	OSLogMaxBytes            int
	OSLogInterval            time.Duration
	OSLogWinChannels         []string
	OSLogEnrich              bool
	OSLogDetections          bool
	OSLogDiag                bool
	AgentlessEnabled         bool
	AgentlessPollInterval    time.Duration
	AgentlessFlushInterval   time.Duration
	SelfHealPollInterval     time.Duration
	AgentlessOutboxPath      string
	AgentlessOutboxMaxMB     int
	AgentlessJobsLimit       int
	AgentlessLockSec         int
	AgentlessFlushBatch      int
	AgentlessDebug           bool
	AutoUpdateEnabled        bool
	AutoUpdateDir            string
	AutoUpdateCommand        string
	AutoUpdateWorkDir        string
	AutoUpdateTimeout        time.Duration
	AutoUpdateRetryInterval  time.Duration
	AutoUpdateMaxMB          int
	AutoUpdateUseAgentAuth   bool
	AgentClientID            int
	AgentID                  int
	AgentInstallationID      string
	AgentIdentitySecret      string
}

type CollectPrefs struct {
	Version                    string   `json:"version,omitempty"`
	Paused                     bool     `json:"paused,omitempty"`
	CPU                        bool     `json:"cpu"`
	Memory                     bool     `json:"memory"`
	Disk                       bool     `json:"disk"`
	Network                    bool     `json:"network"`
	NetActive                  bool     `json:"net_active"`
	Host                       bool     `json:"host"`
	Sensors                    bool     `json:"sensors"`
	Power                      bool     `json:"power"`
	Sanity                     bool     `json:"sanity"`
	GPU                        bool     `json:"gpu"`
	Services                   bool     `json:"services"`
	TimeSync                   bool     `json:"time_sync"`
	Logs                       bool     `json:"logs"`
	Updates                    bool     `json:"updates"`
	Agent                      bool     `json:"agent"`
	Processes                  bool     `json:"processes"`
	Vulns                      bool     `json:"vulns"`
	Inventory                  bool     `json:"inventory"`
	OSLogEnrich                bool     `json:"oslog_enrich"`
	OSLogDetections            bool     `json:"oslog_detections"`
	OSLogDiag                  bool     `json:"oslog_diag"`
	OSLogWinChannels           bool     `json:"oslog_win_channels"`
	OSLogFiles                 bool     `json:"oslog_files"`
	CollectNow                 []string `json:"collect_now,omitempty"`
	CVESignaturesURL           string   `json:"cve_signatures_url,omitempty"`
	OSLogWinChList             []string `json:"oslog_win_channels_list,omitempty"`
	OSLogFilesList             []string `json:"oslog_files_list,omitempty"`
	OSLogBatchLines            int      `json:"oslog_batch_lines,omitempty"`
	OSLogMaxBytes              int      `json:"oslog_max_bytes,omitempty"`
	OSLogIntervalSec           int      `json:"oslog_interval,omitempty"`
	AgentlessEnabled           bool     `json:"agentless_enabled,omitempty"`
	AgentlessPollSec           int      `json:"agentless_poll_interval,omitempty"`
	AgentlessFlushSec          int      `json:"agentless_flush_interval,omitempty"`
	AgentlessJobsLimit         int      `json:"agentless_jobs_limit,omitempty"`
	AgentlessLockSec           int      `json:"agentless_lock_sec,omitempty"`
	AgentlessFlushBatch        int      `json:"agentless_flush_batch,omitempty"`
	NetworkPassiveMode         string   `json:"network_passive_mode,omitempty"`
	NetworkCaptureWindowSec    int      `json:"network_capture_window_sec,omitempty"`
	NetworkCaptureSampleSec    int      `json:"network_capture_sample_sec,omitempty"`
	NetworkCaptureMaxFlows     int      `json:"network_capture_max_flows,omitempty"`
	NetworkCaptureMaxPeers     int      `json:"network_capture_max_peers,omitempty"`
	NetworkCaptureMaxListeners int      `json:"network_capture_max_listeners,omitempty"`
	NetworkCaptureTimeoutMs    int      `json:"network_capture_timeout_ms,omitempty"`
	NetworkPCAPEnabled         bool     `json:"network_pcap_enabled,omitempty"`
	NetworkPCAPIface           string   `json:"network_pcap_iface,omitempty"`
	NetworkPCAPDurationSec     int      `json:"network_pcap_duration_sec,omitempty"`
	NetworkPCAPMaxPackets      int      `json:"network_pcap_max_packets,omitempty"`
	NetworkExternalSources     []string `json:"network_external_sources,omitempty"`
	NetworkExternalMaxRecords  int      `json:"network_external_max_records,omitempty"`
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
		Agent:                  AgentCfg{LogLevel: getenv("LOG_LEVEL", "info"), Token: loadToken()},
		APIBaseURL:             getenv("API_BASE_URL", "https://api.aiceberg.com.br"),
		APIKey:                 getenv("API_KEY", ""),
		HTTPGzip:               strings.ToLower(getenv("HTTP_GZIP", "")) == "true",
		HTTPIdempotency:        strings.ToLower(getenv("HTTP_IDEMPOTENCY", "true")) == "true",
		OutboxFlushBatch:       intEnv("OUTBOX_FLUSH_BATCH", 50),
		TLSInsecureSkip:        strings.ToLower(getenv("TLS_INSECURE_SKIP_VERIFY", "")) == "true",
		OutboxPath:             getenv("OUTBOX_PATH", "./data/outbox.db"),
		OutboxMaxMB:            intEnv("OUTBOX_MAX_MB", 200),
		OutboxMaxPerAgent:      intEnv("OUTBOX_MAX_PER_AGENT", 0),
		HealthPort:             port,
		PrefsPath:              getenv("PREFS_PATH", "./data/collect_prefs.json"),
		AgentMode:              strings.ToLower(getenv("AGENT_MODE", "direct")),
		AgentModeOverridePath:  getenv("AGENT_MODE_OVERRIDE_PATH", ""),
		HubURL:                 getenv("HUB_URL", ""),
		HubToken:               getenv("HUB_TOKEN", ""),
		HubListenAddr:          getenv("HUB_LISTEN_ADDR", ""),
		SkipBootstrap:          strings.ToLower(getenv("SKIP_BOOTSTRAP", "")) == "true",
		OSLogEnabled:           strings.ToLower(getenv("OSLOG_ENABLED", "")) == "true",
		OSLogFiles:             splitCsv(getenv("OSLOG_FILES", "")),
		OSLogCursorPath:        getenv("OSLOG_CURSOR_PATH", "./data/oslogs.cursor"),
		OSLogBatchLines:        intEnv("OSLOG_BATCH_LINES", 200),
		OSLogMaxBytes:          intEnv("OSLOG_MAX_BYTES", 256*1024),
		OSLogInterval:          time.Duration(intEnv("OSLOG_INTERVAL", 15)) * time.Second,
		OSLogWinChannels:       splitCsv(getenv("OSLOG_WIN_CHANNELS", "")),
		OSLogEnrich:            strings.ToLower(getenv("OSLOG_ENRICH", "")) == "true",
		OSLogDetections:        strings.ToLower(getenv("OSLOG_DETECTIONS", "")) == "true",
		OSLogDiag:              strings.ToLower(getenv("OSLOG_DIAG", "")) == "true",
		AgentlessEnabled:       strings.ToLower(getenv("AGENTLESS_ENABLED", "true")) == "true",
		AgentlessOutboxPath:    getenv("AGENTLESS_OUTBOX_PATH", "./data/agentless_outbox.db"),
		AgentlessOutboxMaxMB:   intEnv("AGENTLESS_OUTBOX_MAX_MB", 50),
		AgentlessJobsLimit:     intEnv("AGENTLESS_JOBS_LIMIT", 50),
		AgentlessLockSec:       intEnv("AGENTLESS_LOCK_SEC", 60),
		AgentlessFlushBatch:    intEnv("AGENTLESS_FLUSH_BATCH", 100),
		AgentlessDebug:         strings.ToLower(getenv("AGENTLESS_DEBUG", "")) == "true",
		AutoUpdateEnabled:      strings.ToLower(getenv("AUTO_UPDATE_ENABLED", "")) == "true",
		AutoUpdateDir:          getenv("AUTO_UPDATE_DIR", "./data/updates"),
		AutoUpdateCommand:      getenv("AUTO_UPDATE_COMMAND", ""),
		AutoUpdateWorkDir:      getenv("AUTO_UPDATE_WORKDIR", ""),
		AutoUpdateMaxMB:        intEnv("AUTO_UPDATE_MAX_MB", 300),
		AutoUpdateUseAgentAuth: strings.ToLower(getenv("AUTO_UPDATE_USE_AGENT_AUTH", "")) == "true",
		AgentClientID:          intEnv("AGENT_CLIENT_ID", 0),
		AgentID:                intEnv("AGENT_ID", 0),
		AgentInstallationID:    getenv("AGENT_INSTALLATION_ID", ""),
		AgentIdentitySecret:    getenv("AGENT_IDENTITY_SECRET", ""),
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
