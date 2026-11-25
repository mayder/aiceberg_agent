//go:build linux || windows

package packageretirement

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type packageProbe struct {
	source string
	read   func(context.Context) (map[packageAddress]packageRecord, error)
}

func validatePackageRetirementScope(client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid application discovery scope")
	}
	if strings.TrimSpace(os.Getenv("AICEBERG_AGENT_ENABLE_PACKAGE_INVENTORY")) != "true" {
		return errors.New("application retirement requires package inventory")
	}
	return nil
}

func watchPackageProbes(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error, probes []packageProbe) error {
	trackers := make([]packageRetirementTracker, len(probes))
	for i, probe := range probes {
		trackers[i] = packageRetirementTracker{client: client, asset: asset, source: probe.source, publish: publish}
	}
	poll := func() {
		for i, probe := range probes {
			if probe.read != nil {
				pollPackageInventory(ctx, probe, &trackers[i])
			}
		}
	}
	poll()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			poll()
		}
	}
}

func pollPackageInventory(ctx context.Context, probe packageProbe, tracker *packageRetirementTracker) {
	cycle, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	inventory, err := probe.read(cycle)
	if err != nil {
		tracker.absent = nil
		return
	}
	_ = tracker.observe(cycle, inventory, time.Now())
}
