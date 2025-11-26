package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type AgentCfg struct {
	LogLevel string `json:"log_level"`
	Token    string `json:"token"`
}

type Config struct {
	Agent              AgentCfg
	APIBaseURL         string
	APIKey             string
	HealthPort         int
	PingInterval       time.Duration
	ConfigSyncInterval time.Duration
	PrefsPath          string
}

type CollectPrefs struct {
	Version   string `json:"version,omitempty"`
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

func Load(_ string) (Config, error) {
	port := 0
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	pingInterval := time.Duration(intEnv("PING_INTERVAL", 5)) * time.Second
	cfgSyncInterval := time.Duration(intEnv("CONFIG_SYNC_INTERVAL", 30)) * time.Second
	cfg := Config{
		Agent:      AgentCfg{LogLevel: getenv("LOG_LEVEL", "info"), Token: loadToken()},
		APIBaseURL: getenv("API_BASE_URL", "https://api.aiceberg.com.br"),
		APIKey:     getenv("API_KEY", ""),
		HealthPort: port,
		PrefsPath:  getenv("PREFS_PATH", "./data/collect_prefs.json"),
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

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
