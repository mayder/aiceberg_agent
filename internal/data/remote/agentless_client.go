package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type AgentlessHubClient struct {
	cfg config.Config
	cl  *http.Client
}

type AgentlessFetchOptions struct {
	CommandID     string
	CorrelationID string
	CheckIDs      []int
}

func NewAgentlessHubClient(cfg config.Config) *AgentlessHubClient {
	return &AgentlessHubClient{
		cfg: cfg,
		cl:  httpx.NewClient(cfg, 12*time.Second),
	}
}

func (c *AgentlessHubClient) FetchJobs(ctx context.Context, limit int, lock bool, lockSec int) ([]entities.AgentlessJob, error) {
	return c.FetchJobsWithOptions(ctx, limit, lock, lockSec, AgentlessFetchOptions{})
}

func (c *AgentlessHubClient) FetchJobsWithOptions(ctx context.Context, limit int, lock bool, lockSec int, opts AgentlessFetchOptions) ([]entities.AgentlessJob, error) {
	endpoint := c.cfg.APIEndpoint("/v1/hub-agentless/jobs")
	params := url.Values{}
	params.Set("limit", strconv.Itoa(limit))
	if lock {
		params.Set("lock", "1")
		params.Set("lock_sec", strconv.Itoa(lockSec))
	} else {
		params.Set("lock", "0")
	}
	if commandID := strings.TrimSpace(opts.CommandID); commandID != "" {
		params.Set("command_id", commandID)
	}
	if correlationID := strings.TrimSpace(opts.CorrelationID); correlationID != "" {
		params.Set("correlation_id", correlationID)
	}
	if checkIDs := joinPositiveInts(opts.CheckIDs); checkIDs != "" {
		params.Set("check_ids", checkIDs)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
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
		return nil, fmt.Errorf("jobs http %s body=%s", resp.Status, string(body))
	}
	var out struct {
		Status string                  `json:"status"`
		Count  int                     `json:"count"`
		Jobs   []entities.AgentlessJob `json:"jobs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Jobs, nil
}

func joinPositiveInts(values []int) string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value > 0 {
			out = append(out, strconv.Itoa(value))
		}
	}
	return strings.Join(out, ",")
}

func (c *AgentlessHubClient) SendObservations(ctx context.Context, list []entities.AgentlessObservation) error {
	if len(list) == 0 {
		return nil
	}
	payload := make(map[string]any)
	obs := make([]map[string]any, 0, len(list))
	for _, o := range list {
		item := map[string]any{
			"check_id":     o.CheckID,
			"status":       o.Status,
			"latency_ms":   o.LatencyMs,
			"code":         o.Code,
			"message":      o.Message,
			"payload_json": o.Payload,
			"observed_at":  o.ObservedAt.Format("2006-01-02 15:04:05"),
		}
		if o.CollectionKind != "" {
			item["snmp_collection_kind"] = o.CollectionKind
			item["collection_kind"] = o.CollectionKind
		}
		if o.CommandID != "" {
			item["command_id"] = o.CommandID
		}
		if o.CorrelationID != "" {
			item["correlation_id"] = o.CorrelationID
		}
		if o.SegmentID != "" {
			item["segment_id"] = o.SegmentID
			item["is_partial"] = o.IsPartial
			item["is_final"] = o.IsFinal
		}
		if o.SegmentSeq > 0 {
			item["segment_seq"] = o.SegmentSeq
		}
		if o.SegmentStartedAt != nil && !o.SegmentStartedAt.IsZero() {
			item["segment_started_at"] = o.SegmentStartedAt.Format("2006-01-02 15:04:05")
		}
		if o.DedupeKey != "" {
			item["dedupe_key"] = o.DedupeKey
		}
		if o.EndpointID != nil {
			item["endpoint_id"] = *o.EndpointID
		}
		obs = append(obs, item)
	}
	payload["observations"] = obs
	raw, _ := json.Marshal(payload)

	url := c.cfg.APIEndpoint("/v1/hub-agentless/observations")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	httpx.SetAuth(req, c.cfg)
	resp, err := c.cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("observations http %s body=%s", resp.Status, string(body))
	}
	return nil
}
