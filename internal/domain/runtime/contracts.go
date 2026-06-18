package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const (
	PipelineVersion = "2-compatible"
	PayloadSchemaV1 = 1
)

type Collector interface {
	Name() string
	Version() string
	Capabilities() []string
	Interval() time.Duration
	Timeout() time.Duration
	Collect(context.Context) (Result, error)
}

type Result struct {
	Body        []byte
	CollectedAt time.Time
	Metadata    map[string]any
}

type CollectorSpec struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Endpoint     string        `json:"endpoint"`
	Capabilities []string      `json:"capabilities,omitempty"`
	Interval     time.Duration `json:"interval,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
	Priority     int           `json:"priority,omitempty"`
}

type Forwarder interface {
	Name() string
	Version() string
	Flush(context.Context) (ForwarderResult, error)
	Snapshot() ForwarderSnapshot
}

type ForwarderResult struct {
	Acked      int
	Retained   int
	DurationMs int64
}

type ForwarderSnapshot struct {
	Name                string         `json:"name"`
	Version             string         `json:"version"`
	BatchSize           int            `json:"batch_size,omitempty"`
	BackpressureEnabled bool           `json:"backpressure_enabled"`
	EndpointBacklog     map[string]int `json:"endpoint_backlog,omitempty"`
	LastErrorRoute      string         `json:"last_error_route,omitempty"`
	LastDurationMs      int64          `json:"last_duration_ms,omitempty"`
}

type Scheduler interface {
	Register(CollectorSpec) error
	Snapshot() SchedulerSnapshot
}

type SchedulerSnapshot struct {
	PipelineVersion string          `json:"pipeline_version"`
	Collectors      []CollectorSpec `json:"collectors"`
}

type Supervisor interface {
	Snapshot() SupervisorSnapshot
}

type SupervisorSnapshot struct {
	PipelineVersion string `json:"pipeline_version"`
	Status          string `json:"status"`
	UptimeSec       int64  `json:"uptime_sec,omitempty"`
	CollectErr      int64  `json:"collect_err,omitempty"`
	FlushErr        int64  `json:"flush_err,omitempty"`
	LastCollectMs   int64  `json:"last_collect_ms,omitempty"`
	LastFlushMs     int64  `json:"last_flush_ms,omitempty"`
}

type ExtensionRuntime interface {
	Name() string
	Version() string
	Enabled() bool
	Snapshot() map[string]any
}

func WithPayloadMetadata(raw []byte, collectorName, endpoint string) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("runtime payload metadata: %w", err)
	}
	body["schema_version"] = PayloadSchemaV1
	body["agent_pipeline_version"] = PipelineVersion
	body["collector_name"] = collectorName
	body["ingest_endpoint"] = endpoint
	return json.Marshal(body)
}

func SchedulerSnapshotForCollectors(collectors []CollectorSpec) SchedulerSnapshot {
	out := make([]CollectorSpec, len(collectors))
	copy(out, collectors)
	return SchedulerSnapshot{
		PipelineVersion: PipelineVersion,
		Collectors:      out,
	}
}
