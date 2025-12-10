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
	Agent              AgentCfg
	APIBaseURL         string
	APIKey             string
	HTTPGzip           bool
	HTTPIdempotency    bool
	TLSInsecureSkip    bool
	OutboxPath         string
	OutboxMaxMB        int
	HealthPort         int
	PingInterval       time.Duration
	ConfigSyncInterval time.Duration
	PrefsPath          string
	AgentMode          string
	HubURL             string
	HubToken           string
	HubListenAddr      string
	SkipBootstrap      bool
	OSLogEnabled       bool
	OSLogFiles         []string
	OSLogCursorPath    string
	OSLogBatchLines    int
	OSLogMaxBytes      int
	OSLogInterval      time.Duration
	OSLogWinChannels   []string
	OSLogDiag          bool
}

type CollectPrefs struct {
	Version   string `json:"version,omitempty"`
	Paused    bool   `json:"paused,omitempty"`
	CPU       bool   `json:"cpu"`
	Memory    bool   `json:"memory"`
	Disk      bool   `json:"disk"`
	Network   bool   `json:"network"`
	NetActive bool   `json:"net_active"`
	Host      bool   `json:"host"`
	Sensors   bool   `json:"sensors"`
	Power     bool   `json:"power"`
	Sanity    bool   `json:"sanity"`
	GPU       bool   `json:"gpu"`
	Services  bool   `json:"services"`
	TimeSync  bool   `json:"time_sync"`
	Logs      bool   `json:"logs"`
	Updates   bool   `json:"updates"`
	Agent     bool   `json:"agent"`
	Processes bool   `json:"processes"`
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
	cfg := Config{
		Agent:            AgentCfg{LogLevel: getenv("LOG_LEVEL", "info"), Token: loadToken()},
		APIBaseURL:       getenv("API_BASE_URL", "https://api.aiceberg.com.br"),
		APIKey:           getenv("API_KEY", ""),
		HTTPGzip:         strings.ToLower(getenv("HTTP_GZIP", "")) == "true",
		HTTPIdempotency:  strings.ToLower(getenv("HTTP_IDEMPOTENCY", "true")) == "true",
		TLSInsecureSkip:  strings.ToLower(getenv("TLS_INSECURE_SKIP_VERIFY", "")) == "true",
		OutboxPath:       getenv("OUTBOX_PATH", "./data/outbox.db"),
		OutboxMaxMB:      intEnv("OUTBOX_MAX_MB", 200),
		HealthPort:       port,
		PrefsPath:        getenv("PREFS_PATH", "./data/collect_prefs.json"),
		AgentMode:        strings.ToLower(getenv("AGENT_MODE", "direct")),
		HubURL:           getenv("HUB_URL", ""),
		HubToken:         getenv("HUB_TOKEN", ""),
		HubListenAddr:    getenv("HUB_LISTEN_ADDR", ""),
		SkipBootstrap:    strings.ToLower(getenv("SKIP_BOOTSTRAP", "")) == "true",
		OSLogEnabled:     strings.ToLower(getenv("OSLOG_ENABLED", "")) == "true",
		OSLogFiles:       splitCsv(getenv("OSLOG_FILES", "")),
		OSLogCursorPath:  getenv("OSLOG_CURSOR_PATH", "./data/oslogs.cursor"),
		OSLogBatchLines:  intEnv("OSLOG_BATCH_LINES", 200),
		OSLogMaxBytes:    intEnv("OSLOG_MAX_BYTES", 256*1024),
		OSLogInterval:    time.Duration(intEnv("OSLOG_INTERVAL", 15)) * time.Second,
		OSLogWinChannels: splitCsv(getenv("OSLOG_WIN_CHANNELS", "")),
		OSLogDiag:        strings.ToLower(getenv("OSLOG_DIAG", "")) == "true",
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
