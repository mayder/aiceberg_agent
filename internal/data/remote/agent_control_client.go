package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type AgentControlClient struct {
	cfg config.Config
	cl  *http.Client
}

func NewAgentControlClient(cfg config.Config) *AgentControlClient {
	return &AgentControlClient{
		cfg: cfg,
		cl:  httpx.NewClient(cfg, 12*time.Second),
	}
}

func (c *AgentControlClient) FetchSelfHealCommands(ctx context.Context) ([]entities.SelfHealCommand, error) {
	url := c.cfg.APIEndpoint("/v1/agent/selfheal-commands")
	if c.cfg.AgentMode == "relay" && c.cfg.HubURL != "" {
		url = strings.TrimRight(c.cfg.HubURL, "/") + "/v1/agent/selfheal-commands"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpx.SetAuth(req, c.cfg)
	resp, err := c.cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("selfheal-commands http %s body=%s", resp.Status, string(body))
	}
	var out struct {
		Status   string                     `json:"status"`
		Commands []entities.SelfHealCommand `json:"commands"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Commands, nil
}

func (c *AgentControlClient) ReportSelfHeal(ctx context.Context, report entities.SelfHealReport) error {
	if strings.TrimSpace(report.CommandID) == "" || strings.TrimSpace(report.Status) == "" {
		return fmt.Errorf("invalid selfheal report")
	}
	if report.ReportedAtMs <= 0 {
		report.ReportedAtMs = time.Now().UnixMilli()
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return c.postWithFallback(ctx, "/v1/agent/selfheal-report", raw)
}

func (c *AgentControlClient) ReportWorkerErrors(ctx context.Context, list []entities.WorkerErrorEvent) error {
	if len(list) == 0 {
		return nil
	}
	payload := map[string]any{"errors": list}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.postWithFallback(ctx, "/v1/agent/error-report", raw)
}

func (c *AgentControlClient) postWithFallback(ctx context.Context, path string, raw []byte) error {
	urls := []string{}
	if c.cfg.AgentMode == "relay" && strings.TrimSpace(c.cfg.HubURL) != "" {
		urls = append(urls, strings.TrimRight(strings.TrimSpace(c.cfg.HubURL), "/")+path)
	}
	urls = append(urls, c.cfg.APIEndpoint(path))

	var errs []string
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		httpx.SetAuth(req, c.cfg)
		resp, err := c.cl.Do(req)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s status=%d", url, resp.StatusCode))
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, " | "))
}
