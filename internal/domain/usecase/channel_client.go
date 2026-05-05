package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/version"
	"github.com/you/aiceberg_agent/internal/domain/channel"
)

const channelRoute = "/v1/agent/channel"

type AgentChannelClient struct {
	cfg           config.Config
	log           logger.Logger
	cl            *http.Client
	hostname      string
	capabilities  []string
	newSessionID  func(string) string
	handleCommand func(context.Context, channel.Envelope) error
	mu            sync.RWMutex
	status        ChannelRuntimeStatus
}

type ChannelRuntimeStatus struct {
	Enabled               bool   `json:"enabled"`
	Mode                  string `json:"mode"`
	Endpoint              string `json:"endpoint,omitempty"`
	FallbackActive        bool   `json:"fallback_active"`
	Connected             bool   `json:"connected"`
	SessionID             string `json:"session_id,omitempty"`
	LastHeartbeatUTC      string `json:"last_heartbeat_utc,omitempty"`
	LastError             string `json:"last_error,omitempty"`
	ReconnectRetries      int    `json:"reconnect_retries"`
	LastLatencyMs         int64  `json:"last_latency_ms,omitempty"`
	RelayUsesHubURL       bool   `json:"relay_uses_hub_url,omitempty"`
	HubURLConfigured      bool   `json:"hub_url_configured,omitempty"`
	ConnectsToAiceberg    bool   `json:"connects_to_aiceberg"`
	RelayConnectsAiceberg bool   `json:"relay_connects_to_aiceberg"`
}

func NewAgentChannelClient(cfg config.Config, log logger.Logger) *AgentChannelClient {
	hostname, _ := os.Hostname()
	client := &AgentChannelClient{
		cfg:          cfg,
		log:          log,
		cl:           httpx.NewClient(cfg, 10*time.Second),
		hostname:     hostname,
		capabilities: DefaultChannelCapabilities(cfg.Mode()),
		newSessionID: randomChannelSessionID,
	}
	client.initStatus()
	return client
}

func (c *AgentChannelClient) SetCommandHandler(handler func(context.Context, channel.Envelope) error) {
	c.handleCommand = handler
}

func (c *AgentChannelClient) SendEvent(ctx context.Context, env channel.Envelope) error {
	if strings.TrimSpace(env.ContractID) == "" {
		env.ContractID = channel.ContractID
	}
	if env.SchemaVersion == 0 {
		env.SchemaVersion = channel.SchemaVersion
	}
	if strings.TrimSpace(env.MessageID) == "" {
		env.MessageID = randomChannelSessionID("msg")
	}
	if strings.TrimSpace(env.TimestampUTC) == "" {
		env.TimestampUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if strings.TrimSpace(env.AgentID) == "" {
		if hostname, err := os.Hostname(); err == nil {
			env.AgentID = hostname
		}
	}
	if strings.TrimSpace(env.Mode) == "" {
		env.Mode = c.cfg.Mode()
	}
	payload := map[string]any{
		"action":         "event",
		"session_id":     randomChannelSessionID(c.cfg.Mode()),
		"mode":           c.cfg.Mode(),
		"agent_version":  version.Version,
		"contract_id":    env.ContractID,
		"schema_version": env.SchemaVersion,
		"message_id":     env.MessageID,
		"type":           env.Type,
		"timestamp_utc":  env.TimestampUTC,
		"agent_id":       env.AgentID,
		"hub_agent_id":   env.HubAgentID,
		"command_id":     env.CommandID,
		"correlation_id": env.CorrelationID,
		"attempt":        env.Attempt,
		"timeout_ms":     env.TimeoutMs,
		"retry_after_ms": env.RetryAfterMs,
		"payload":        env.Payload,
		"progress":       env.Progress,
		"result":         env.Result,
		"error":          env.Error,
	}
	_, err := c.post(ctx, payload)
	return err
}

func DefaultChannelCapabilities(mode string) []string {
	capabilities := []string{
		"ping",
		"bootstrap",
		"config",
		"selfheal",
		"update-report",
		"ingest",
	}
	switch mode {
	case channel.ModeHub:
		return append(capabilities, "channel.hub", "relay.command.receive")
	case channel.ModeRelay:
		return append(capabilities, "channel.relay")
	default:
		return append(capabilities, "channel.direct")
	}
}

func (c *AgentChannelClient) Run(ctx context.Context) {
	mode := c.cfg.Mode()
	if mode == channel.ModeRelay && c.cfg.HubURL == "" {
		c.recordError(mode, "", fmt.Errorf("missing HUB_URL"))
		c.log.Error(logger.KV("channel relay disabled",
			"route", channelRoute,
			"mode", mode,
			"err", "missing HUB_URL",
		))
		return
	}
	if mode != channel.ModeRelay && !channel.ModeConnectsToAiceberg(mode) {
		c.recordError(mode, "", fmt.Errorf("invalid channel mode"))
		return
	}
	c.run(ctx, mode)
}

func (c *AgentChannelClient) RunDirect(ctx context.Context) {
	if c.cfg.Mode() != channel.ModeDirect {
		return
	}
	c.run(ctx, channel.ModeDirect)
}

func (c *AgentChannelClient) RunHub(ctx context.Context) {
	if c.cfg.Mode() != channel.ModeHub {
		return
	}
	c.run(ctx, channel.ModeHub)
}

func (c *AgentChannelClient) RunRelay(ctx context.Context) {
	if c.cfg.Mode() != channel.ModeRelay || c.cfg.HubURL == "" {
		return
	}
	c.run(ctx, channel.ModeRelay)
}

func (c *AgentChannelClient) run(ctx context.Context, mode string) {
	backoff := newChannelBackoff()
	for ctx.Err() == nil {
		sessionID := c.newSessionID(mode)
		latencyMs, err := c.open(ctx, sessionID, mode)
		if err != nil {
			c.recordError(mode, sessionID, err)
			c.log.Error(logger.KV("channel open failed",
				"route", channelRoute,
				"mode", mode,
				"fallback", "polling",
				"err", err,
			))
			if !sleepContext(ctx, backoff.Next()) {
				return
			}
			continue
		}
		backoff.Reset()
		c.recordConnected(mode, sessionID, latencyMs)
		c.log.Info(logger.KV("channel opened",
			"route", channelRoute,
			"mode", mode,
			"session_id", sessionID,
			"duration_ms", latencyMs,
		))
		if err := c.heartbeatLoop(ctx, sessionID, mode, latencyMs); err != nil && ctx.Err() == nil {
			c.recordError(mode, sessionID, err)
			c.log.Error(logger.KV("channel reconnect scheduled",
				"route", channelRoute,
				"mode", mode,
				"session_id", sessionID,
				"fallback", "polling",
				"err", err,
			))
			if !sleepContext(ctx, backoff.Next()) {
				return
			}
			continue
		}
		_ = c.close(context.WithoutCancel(ctx), sessionID, mode)
		c.recordClosed(mode, sessionID)
		return
	}
}

func (c *AgentChannelClient) heartbeatLoop(ctx context.Context, sessionID, mode string, lastLatencyMs int64) error {
	interval := c.cfg.ChannelHeartbeatInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			latencyMs, err := c.heartbeat(ctx, sessionID, mode, lastLatencyMs)
			if err != nil {
				return err
			}
			lastLatencyMs = latencyMs
			c.recordHeartbeat(mode, sessionID, latencyMs)
		}
	}
}

func (c *AgentChannelClient) open(ctx context.Context, sessionID, mode string) (int64, error) {
	return c.post(ctx, map[string]any{
		"action":       "open",
		"session_id":   sessionID,
		"mode":         mode,
		"version":      version.Version,
		"hostname":     c.hostname,
		"capabilities": c.capabilities,
	})
}

func (c *AgentChannelClient) heartbeat(ctx context.Context, sessionID, mode string, lastLatencyMs int64) (int64, error) {
	return c.post(ctx, map[string]any{
		"action":       "heartbeat",
		"session_id":   sessionID,
		"mode":         mode,
		"version":      version.Version,
		"hostname":     c.hostname,
		"capabilities": c.capabilities,
		"latency_ms":   lastLatencyMs,
	})
}

func (c *AgentChannelClient) close(ctx context.Context, sessionID, mode string) error {
	_, err := c.post(ctx, map[string]any{
		"action":     "close",
		"session_id": sessionID,
		"mode":       mode,
		"version":    version.Version,
		"hostname":   c.hostname,
	})
	return err
}

func (c *AgentChannelClient) post(ctx context.Context, payload map[string]any) (int64, error) {
	if identity := c.cfg.AgentIdentityClaim(""); len(identity) > 0 {
		payload["agent_identity"] = identity
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.channelEndpoint(), bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	httpx.SetAuth(req, c.cfg)
	if identityHeader := c.cfg.AgentIdentityHeader(""); identityHeader != "" {
		req.Header.Set("X-Agent-Identity", identityHeader)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.cl.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	latencyMs := time.Since(start).Milliseconds()
	if resp.StatusCode >= 300 {
		return latencyMs, fmt.Errorf("channel http %s", resp.Status)
	}
	var response channelServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err == nil {
		c.handleServerCommands(ctx, response.Commands)
		c.handleServerCommands(ctx, response.Envelopes)
	}
	return latencyMs, nil
}

func (c *AgentChannelClient) channelEndpoint() string {
	if c.cfg.Mode() == channel.ModeRelay && c.cfg.HubURL != "" {
		return strings.TrimRight(c.cfg.HubURL, "/") + channelRoute
	}
	return c.cfg.APIEndpoint(channelRoute)
}

type channelServerResponse struct {
	Commands  []channel.Envelope `json:"commands,omitempty"`
	Envelopes []channel.Envelope `json:"envelopes,omitempty"`
}

func (c *AgentChannelClient) handleServerCommands(ctx context.Context, commands []channel.Envelope) {
	if len(commands) == 0 {
		return
	}
	accepted := 0
	for _, command := range commands {
		if command.Type != channel.TypeCommand {
			continue
		}
		if errors := channel.ValidateEnvelope(command); len(errors) > 0 {
			c.log.Error(logger.KV("channel command rejected",
				"route", channelRoute,
				"command_id", command.CommandID,
				"err", fmt.Sprint(errors),
			))
			continue
		}
		accepted++
		if c.handleCommand != nil {
			if err := c.handleCommand(ctx, command); err != nil {
				c.log.Error(logger.KV("channel command handler failed",
					"route", channelRoute,
					"command_id", command.CommandID,
					"mode", command.Mode,
					"err", err,
				))
			}
		}
	}
	if accepted > 0 {
		c.log.Info(logger.KV("channel commands received",
			"route", channelRoute,
			"count", accepted,
		))
	}
}

type channelBackoff struct {
	attempt int
}

func newChannelBackoff() *channelBackoff {
	return &channelBackoff{}
}

func (b *channelBackoff) Reset() {
	b.attempt = 0
}

func (b *channelBackoff) Next() time.Duration {
	b.attempt++
	delay := time.Second << min(b.attempt-1, 5)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	jitter := time.Duration(b.attempt%5) * 100 * time.Millisecond
	return delay + jitter
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func randomChannelSessionID(mode string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s-%d", mode, time.Now().UnixNano())
	}
	return mode + "-" + hex.EncodeToString(raw[:])
}

func (c *AgentChannelClient) Snapshot() ChannelRuntimeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *AgentChannelClient) initStatus() {
	mode := c.cfg.Mode()
	topology, _ := channel.TopologyForMode(mode)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = ChannelRuntimeStatus{
		Enabled:               mode == channel.ModeDirect || mode == channel.ModeHub || mode == channel.ModeRelay,
		Mode:                  mode,
		Endpoint:              c.channelEndpoint(),
		FallbackActive:        true,
		Connected:             false,
		HubURLConfigured:      strings.TrimSpace(c.cfg.HubURL) != "",
		RelayUsesHubURL:       mode == channel.ModeRelay && strings.TrimSpace(c.cfg.HubURL) != "",
		ConnectsToAiceberg:    topology.ConnectsDirectlyAiceberg,
		RelayConnectsAiceberg: topology.RelayConnectsToAiceberg,
	}
}

func (c *AgentChannelClient) recordConnected(mode, sessionID string, latencyMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Mode = mode
	c.status.SessionID = sessionID
	c.status.Connected = true
	c.status.FallbackActive = false
	c.status.LastError = ""
	c.status.LastLatencyMs = latencyMs
	c.status.LastHeartbeatUTC = time.Now().UTC().Format(time.RFC3339Nano)
}

func (c *AgentChannelClient) recordHeartbeat(mode, sessionID string, latencyMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Mode = mode
	c.status.SessionID = sessionID
	c.status.Connected = true
	c.status.FallbackActive = false
	c.status.LastError = ""
	c.status.LastLatencyMs = latencyMs
	c.status.LastHeartbeatUTC = time.Now().UTC().Format(time.RFC3339Nano)
}

func (c *AgentChannelClient) recordError(mode, sessionID string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Mode = mode
	c.status.SessionID = sessionID
	c.status.Connected = false
	c.status.FallbackActive = true
	c.status.ReconnectRetries++
	if err != nil {
		c.status.LastError = err.Error()
	}
}

func (c *AgentChannelClient) recordClosed(mode, sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.Mode = mode
	c.status.SessionID = sessionID
	c.status.Connected = false
	c.status.FallbackActive = true
}
