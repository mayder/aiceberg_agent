package httpx

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func VerifiedHTTPS(endpoint string, insecureSkipVerify bool) bool {
	parsed, err := url.Parse(endpoint)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" && !insecureSkipVerify
}

// NewClient cria um http.Client respeitando proxy do ambiente e opções de TLS.
func NewClient(cfg config.Config, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.TLSInsecureSkip,
			MinVersion:         tls.VersionTLS12,
		},
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}
}
