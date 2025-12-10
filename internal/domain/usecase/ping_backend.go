package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
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
		cl:       httpx.NewClient(cfg, 5*time.Second),
		hostname: hn,
	}
}

func (uc *PingBackend) Execute(ctx context.Context) error {
	start := time.Now()
	challenge, err := uc.fetchChallenge(ctx)
	if err != nil || challenge == "" {
		if err != nil {
			uc.log.Error("ping challenge failed: " + err.Error())
		}
		return err
	}
	err = uc.sendAck(ctx, challenge)
	if err != nil {
		uc.log.Error("ping ack failed: " + err.Error())
		return err
	}
	uc.log.Info("ping ack sent challenge=" + challenge + " duration_ms=" + strconv.FormatInt(time.Since(start).Milliseconds(), 10))
	return nil
}

func (uc *PingBackend) fetchChallenge(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uc.cfg.APIEndpoint("/v1/agent/ping"), nil)
	if err != nil {
		return "", err
	}
	httpx.SetAuth(req, uc.cfg)

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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uc.cfg.APIEndpoint("/v1/agent/ping"), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	httpx.SetAuth(req, uc.cfg)
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
