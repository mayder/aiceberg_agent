package config

import (
	"os"
	"strconv"
)

type AgentCfg struct {
	LogLevel string `json:"log_level"`
}

type Config struct {
	Agent      AgentCfg
	APIBaseURL string
	APIKey     string
	HealthPort int
}

func Load(_ string) (Config, error) {
	port := 0
	if v := os.Getenv("HEALTH_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}
	return Config{
		Agent:      AgentCfg{LogLevel: getenv("LOG_LEVEL", "info")},
		APIBaseURL: getenv("API_BASE_URL", "http://localhost:8080"),
		APIKey:     getenv("API_KEY", ""),
		HealthPort: port,
	}, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
