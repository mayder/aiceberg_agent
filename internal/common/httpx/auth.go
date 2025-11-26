package httpx

import (
	"net/http"

	"github.com/you/aiceberg_agent/internal/common/config"
)

// SetAuth adiciona header Authorization com token ou API key, se presentes.
func SetAuth(req *http.Request, cfg config.Config) {
	if cfg.Agent.Token != "" {
		req.Header.Set("Authorization", "Token "+cfg.Agent.Token)
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
}
