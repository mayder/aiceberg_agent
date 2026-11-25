package interfaceretirement

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const (
	eventFreshnessSeconds = 300
	eventBatchLimit       = 16
)

type removal struct {
	mac      net.HardwareAddr
	observed time.Time
}

// Run emits only explicit operating-system removals. The existing outbox owns
// retries after publish has durably enqueued the batch.
func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid interface discovery scope")
	}
	var window time.Time
	remaining := 0
	return watchRemovals(ctx, func(removals []removal) {
		now := time.Now()
		if window.IsZero() || now.Sub(window) >= time.Minute {
			window, remaining = now, eventBatchLimit
		}
		if len(removals) <= remaining {
			remaining -= publishRetirements(ctx, client, asset, removals, publish)
		}
	})
}

func removedInterfaces(client, asset int64, removals []removal, interfaces []net.Interface, now time.Time) []monitoring.DiscoveryEvent {
	if len(removals) > eventBatchLimit {
		return nil
	}
	present := make(map[string]bool, len(interfaces))
	for _, iface := range interfaces {
		present[iface.HardwareAddr.String()] = true
	}
	events := make([]monitoring.DiscoveryEvent, 0, len(removals))
	for _, removal := range removals {
		mac := removal.mac.String()
		if present[mac] || removal.observed.After(now) || now.Sub(removal.observed) >= eventFreshnessSeconds*time.Second {
			continue
		}
		present[mac] = true
		event, err := monitoring.NewDiscoveryEvent(client, asset, "interfaces", []string{mac}, "tombstone", 100, eventFreshnessSeconds, removal.observed)
		if err == nil {
			events = append(events, event)
		}
	}
	return events
}

func publishRetirements(ctx context.Context, client, asset int64, removals []removal, publish func(context.Context, []monitoring.DiscoveryEvent) error) int {
	backoff := time.NewTicker(5 * time.Second)
	defer backoff.Stop()
	for ctx.Err() == nil {
		interfaces, err := net.Interfaces()
		if err != nil {
			return 0
		}
		events := removedInterfaces(client, asset, removals, interfaces, time.Now())
		if len(events) == 0 || publish(ctx, events) == nil {
			return len(events)
		}
		select {
		case <-ctx.Done():
			return 0
		case <-backoff.C:
		}
	}
	return 0
}
