package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/common/version"
)

type PingBackend struct {
	cfg      config.Config
	log      logger.Logger
	cl       *http.Client
	hostname string
	backoff  *retry.Backoff
}

func NewPingBackend(cfg config.Config, log logger.Logger) *PingBackend {
	hn, _ := os.Hostname()
	return &PingBackend{
		cfg:      cfg,
		log:      log,
		cl:       httpx.NewClient(cfg, 5*time.Second),
		hostname: hn,
		backoff:  retry.NewBackoff(),
	}
}

func (uc *PingBackend) Execute(ctx context.Context) error {
	if until, ok := uc.backoff.Active(); ok {
		uc.log.Info(logger.KV("ping backoff active",
			"route", "/v1/agent/ping",
			"retry_after_ms", time.Until(until).Milliseconds(),
		))
		return nil
	}
	start := time.Now()
	challenge, err := uc.fetchChallenge(ctx)
	if err != nil || challenge == "" {
		if err != nil {
			uc.recordFailure(err)
			uc.log.Error(logger.KV("ping challenge failed",
				"route", "/v1/agent/ping",
				"err", err,
			))
		}
		if err == nil {
			uc.backoff.Reset()
		}
		return err
	}
	err = uc.sendAck(ctx, challenge)
	if err != nil {
		uc.recordFailure(err)
		uc.log.Error(logger.KV("ping ack failed",
			"route", "/v1/agent/ping",
			"err", err,
		))
		return err
	}
	durationMs := time.Since(start).Milliseconds()
	uc.log.Info(logger.KV("ping ack sent",
		"route", "/v1/agent/ping",
		"challenge", challenge,
		"duration_ms", durationMs,
	))
	uc.backoff.Reset()
	return nil
}

func (uc *PingBackend) recordFailure(err error) {
	delay, kind := uc.backoff.Failure(err)
	if kind != retry.ErrorKindTransient {
		delay = uc.backoff.Cooldown(retry.DefaultMaxBackoff)
		uc.log.Error(logger.KV("ping permanent failure",
			"route", "/v1/agent/ping",
			"err_kind", kind,
			"retry_after_ms", delay.Milliseconds(),
			"err", err,
		))
		return
	}
	uc.log.Info(logger.KV("ping backoff scheduled",
		"route", "/v1/agent/ping",
		"err_kind", kind,
		"retry_after_ms", delay.Milliseconds(),
	))
}

func (uc *PingBackend) fetchChallenge(ctx context.Context) (string, error) {
	url := uc.cfg.APIEndpoint("/v1/agent/ping")
	if uc.cfg.AgentMode == "relay" && uc.cfg.HubURL != "" {
		url = uc.cfg.HubURL + "/v1/agent/ping"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	httpx.SetAuth(req, uc.cfg)
	if identityHeader := uc.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}

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
	if identity := uc.cfg.AgentIdentityClaim(""); len(identity) > 0 {
		body["agent_identity"] = identity
	}
	raw, _ := json.Marshal(body)

	url := uc.cfg.APIEndpoint("/v1/agent/ping")
	if uc.cfg.AgentMode == "relay" && uc.cfg.HubURL != "" {
		url = uc.cfg.HubURL + "/v1/agent/ping"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	httpx.SetAuth(req, uc.cfg)
	if identityHeader := uc.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := uc.cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &httpStatusErr{code: resp.StatusCode}
	}
	uc.log.Info(logger.KV("ping ack sent",
		"route", "/v1/agent/ping",
		"challenge", challenge,
	))
	return nil
}

type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return http.StatusText(e.code) }
func (e *httpStatusErr) StatusCode() int {
	return e.code
}
