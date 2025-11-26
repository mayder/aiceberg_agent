package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
)

type PingBackend struct {
	cfg      config.Config
	log      logger.Logger
	cl       *http.Client
	hostname string
}

func NewPingBackend(cfg config.Config, log logger.Logger) *PingBackend {
	hn, _ := os.Hostname()
	return &PingBackend{
		cfg:      cfg,
		log:      log,
		cl:       &http.Client{Timeout: 5 * time.Second},
		hostname: hn,
	}
}

func (uc *PingBackend) Execute(ctx context.Context) error {
	challenge, err := uc.fetchChallenge(ctx)
	if err != nil || challenge == "" {
		return err
	}
	return uc.sendAck(ctx, challenge)
}

func (uc *PingBackend) fetchChallenge(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uc.cfg.APIBaseURL+"/v1/agent/ping", nil)
	if err != nil {
		return "", err
	}
	applyAuth(req, uc.cfg)

	resp, err := uc.cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}
	if resp.StatusCode >= 300 {
		return "", &httpStatusErr{code: resp.StatusCode}
	}

	var payload struct {
		Challenge string `json:"challenge"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.Challenge, nil
}

func (uc *PingBackend) sendAck(ctx context.Context, challenge string) error {
	body := map[string]any{
		"challenge": challenge,
		"hostname":  uc.hostname,
		"version":   version.Version,
		"sent_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}
	raw, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uc.cfg.APIBaseURL+"/v1/agent/ping", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	applyAuth(req, uc.cfg)
	req.Header.Set("Content-Type", "application/json")

	resp, err := uc.cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &httpStatusErr{code: resp.StatusCode}
	}
	uc.log.Info("ping ack sent challenge=" + challenge)
	return nil
}

func applyAuth(req *http.Request, cfg config.Config) {
	if cfg.Agent.Token != "" {
		req.Header.Set("Authorization", "Token "+cfg.Agent.Token)
	} else if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
}

type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return http.StatusText(e.code) }
