package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/platform/collectors/diskretirement"
	"github.com/you/aiceberg_agent/internal/platform/collectors/interfaceretirement"
	"github.com/you/aiceberg_agent/internal/platform/collectors/packageretirement"
	"github.com/you/aiceberg_agent/internal/platform/collectors/sensorretirement"
	"github.com/you/aiceberg_agent/internal/platform/collectors/serviceretirement"
)

type discoveryRetirementBatch struct {
	name   string
	events []monitoring.DiscoveryEvent
}

func (batch *discoveryRetirementBatch) Name() string      { return batch.name }
func (*discoveryRetirementBatch) Interval() time.Duration { return 0 }

func (batch *discoveryRetirementBatch) Collect(context.Context) ([]byte, error) {
	return json.Marshal(map[string]any{"discovery_events": batch.events})
}

func startDiscoveryRetirements(ctx context.Context, mode, endpoint, auth string, identity func() string, outbox ports.OutboxRepo, log logger.Logger) {
	if mode == "relay" {
		return
	}
	startDiscoveryRetirement(ctx, endpoint, auth, identity, outbox, log, "interface_discovery", "DISCOVERY_INTERFACE", interfaceretirement.Run)
	startDiscoveryRetirement(ctx, endpoint, auth, identity, outbox, log, "disk_discovery", "DISCOVERY_DISK", diskretirement.Run)
	startDiscoveryRetirement(ctx, endpoint, auth, identity, outbox, log, "sensor_discovery", "DISCOVERY_SENSOR", sensorretirement.Run)
	startDiscoveryRetirement(ctx, endpoint, auth, identity, outbox, log, "service_discovery", "DISCOVERY_SERVICE", serviceretirement.Run)
	startDiscoveryRetirement(ctx, endpoint, auth, identity, outbox, log, "application_discovery", "DISCOVERY_APPLICATION", packageretirement.Run)
}

func startDiscoveryRetirement(ctx context.Context, endpoint, auth string, identity func() string, outbox ports.OutboxRepo,
	log logger.Logger, name, envPrefix string, run func(context.Context, int64, int64, func(context.Context, []monitoring.DiscoveryEvent) error) error,
) {
	client, asset, enabled := discoveryRetirementScope(envPrefix)
	if !enabled {
		return
	}
	publish := discoveryRetirementPublisher(endpoint, auth, identity, outbox, log, name)
	go func() {
		err := run(ctx, client, asset, publish)
		if err != nil && ctx.Err() == nil {
			log.Error(logger.KV("discovery retirement stopped", "collector", name, "error", err))
		}
	}()
}

func discoveryRetirementScope(prefix string) (int64, int64, bool) {
	client, clientErr := strconv.ParseInt(os.Getenv(prefix+"_CLIENT_ID"), 10, 64)
	asset, assetErr := strconv.ParseInt(os.Getenv(prefix+"_ASSET_ID"), 10, 64)
	return client, asset, clientErr == nil && assetErr == nil && client > 0 && asset > 0
}

func discoveryRetirementPublisher(endpoint, auth string, identity func() string, outbox ports.OutboxRepo,
	log logger.Logger, name string,
) func(context.Context, []monitoring.DiscoveryEvent) error {
	batch := &discoveryRetirementBatch{name: name}
	collector := usecase.NewCollectAndBufferWithIdentityProvider(batch, outbox, log, auth, identity, endpoint)
	return func(ctx context.Context, events []monitoring.DiscoveryEvent) error {
		batch.events = events
		result, err := collector.ExecuteDetailed(ctx)
		if err == nil && result == nil {
			return fmt.Errorf("%s batch was not buffered", name)
		}
		return err
	}
}
