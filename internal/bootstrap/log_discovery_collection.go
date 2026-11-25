package app

import (
	"context"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/collectors/logdiscovery"
)

type logDiscoveryCollection struct {
	collect    *usecase.CollectAndBuffer
	retirement *logdiscovery.SourceRetirement
	publish    func(context.Context, []monitoring.DiscoveryEvent) error
	log        logger.Logger
}

func newLogDiscoveryCollection(collector ports.Collector, mode, endpoint, auth string, identity func() string,
	outbox ports.OutboxRepo, log logger.Logger,
) *logDiscoveryCollection {
	collection := &logDiscoveryCollection{
		collect: usecase.NewCollectAndBufferWithIdentityProvider(collector, outbox, log, auth, identity, endpoint),
		log:     log,
	}
	client, asset, enabled := discoveryRetirementScope("DISCOVERY_LOG_SOURCE")
	if enabled && mode != "relay" {
		collection.retirement = logdiscovery.NewSourceRetirement(client, asset)
		collection.publish = discoveryRetirementPublisher(endpoint, auth, identity, outbox, log, "log_source_retirement")
	}
	return collection
}

func (c *logDiscoveryCollection) Execute(ctx context.Context) error {
	_, err := c.ExecuteDetailed(ctx)
	return err
}

func (c *logDiscoveryCollection) ExecuteDetailed(ctx context.Context) (*usecase.BufferedCollectResult, error) {
	result, err := c.collect.ExecuteDetailed(ctx)
	if err != nil || c.retirement == nil {
		return result, err
	}
	var body []byte
	if result != nil {
		body = result.Body
	}
	if err := c.retirement.ObserveBuffered(ctx, body, time.Now(), c.publish); err != nil && ctx.Err() == nil {
		c.log.Error(logger.KV("log source retirement failed", "error", err))
	}
	return result, nil
}
