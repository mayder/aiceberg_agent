package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type AgentCfg struct {
	LogLevel string `json:"log_level"`
	Token    string `json:"token"`
}

type Config struct {
	Agent        AgentCfg
	APIBaseURL   string
	APIKey       string
	HealthPort   int
	PingInterval time.Duration
}

func Load(_ string) (Config, error) {
	port := 0
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	pingInterval := time.Duration(intEnv("PING_INTERVAL", 5)) * time.Second
	cfg := Config{
		Agent:      AgentCfg{LogLevel: getenv("LOG_LEVEL", "info"), Token: loadToken()},
		APIBaseURL: getenv("API_BASE_URL", "http://localhost:8080"),
		APIKey:     getenv("API_KEY", ""),
		HealthPort: port,
		PingInterval: func() time.Duration {
			if pingInterval <= 0 {
				return 5 * time.Second
			}
			return pingInterval
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

func intEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
