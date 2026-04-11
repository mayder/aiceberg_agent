package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/common/httpx"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type AgentlessHubClient struct {
	cfg config.Config
	cl  *http.Client
}

func NewAgentlessHubClient(cfg config.Config) *AgentlessHubClient {
	return &AgentlessHubClient{
		cfg: cfg,
		cl:  httpx.NewClient(cfg, 12*time.Second),
	}
}

func (c *AgentlessHubClient) FetchJobs(ctx context.Context, limit int, lock bool, lockSec int) ([]entities.AgentlessJob, error) {
	url := c.cfg.APIEndpoint("/v1/hub-agentless/jobs")
	params := "?limit=" + strconv.Itoa(limit)
	if lock {
		params += "&lock=1&lock_sec=" + strconv.Itoa(lockSec)
	} else {
		params += "&lock=0"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+params, nil)
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
