package sysmetrics

import (
	"context"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/ports"
)

type collector struct{}

func New() ports.Collector { return &collector{} }

func (c *collector) Name() string { return "sysmetrics" }

func (c *collector) Interval() time.Duration { return 10 * time.Second }

func (c *collector) Collect(ctx context.Context) ([]byte, error) {
	return []byte(`{"cpu_percent":0.1,"mem_used_mb":42}`), nil
}
