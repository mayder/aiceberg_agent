package sensorretirement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/collectors/kernelnetlink"
	"golang.org/x/sys/unix"
)

const (
	sysRoot          = "/sys"
	freshnessSeconds = 300
	retirementLimit  = 16
)

type hwmonEvent struct {
	action   string
	path     string
	observed time.Time
}

type sensorRetirements struct {
	root          string
	client, asset int64
	publish       func(context.Context, []monitoring.DiscoveryEvent) error
	pending       []sensorRemoval
}

type sensorCandidate struct {
	key   string
	event monitoring.DiscoveryEvent
}

type sensorRemoval struct {
	path     string
	observed time.Time
	sensors  []sensorCandidate
}

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid sensor discovery scope")
	}
	if root := os.Getenv("HOST_SYS"); root != "" && filepath.Clean(root) != sysRoot {
		return errors.New("sensor retirement requires the local kernel inventory")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	events, stopped := watchHwmon(ctx)
	collector := sensorRetirements{root: sysRoot, client: client, asset: asset, publish: publish}
	previous, _ := readInventory(ctx, sysRoot)
	return collector.watch(ctx, previous, events, stopped)
}

func watchHwmon(ctx context.Context) (<-chan hwmonEvent, <-chan error) {
	events := make(chan hwmonEvent, retirementLimit)
	stopped := make(chan error, 1)
	go func() {
		stopped <- kernelnetlink.Receive(ctx, unix.NETLINK_KOBJECT_UEVENT, 1, func(raw []byte) {
			properties := kernelnetlink.DeviceEvent(raw, "hwmon")
			path := properties["DEVPATH"]
			if hwmonName.MatchString(filepath.Base(path)) {
				select {
				case events <- hwmonEvent{properties["ACTION"], path, time.Now()}:
				case <-ctx.Done():
				}
			}
		})
	}()
	return events, stopped
}

func (r *sensorRetirements) watch(ctx context.Context, previous sensorInventory, events <-chan hwmonEvent, stopped <-chan error) error {
	refresh := time.NewTicker(15 * time.Second)
	defer refresh.Stop()
	retry := time.NewTicker(5 * time.Second)
	defer retry.Stop()
	var rateWindow time.Time
	budget := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-stopped:
			return err
		case <-refresh.C:
			// Do not replace the last inventory ahead of a queued removal.
			if len(events) == 0 {
				previous, _ = readInventory(ctx, r.root)
			}
		case <-retry.C:
			if len(events) == 0 {
				r.retryPending(ctx, time.Now())
			}
		case event := <-events:
			if event.action == "add" || (event.action == "remove" && len(previous.byDevice[event.path]) > 0) {
				r.pending = slices.DeleteFunc(r.pending, func(batch sensorRemoval) bool { return batch.path == event.path })
			}
			if event.observed.Sub(rateWindow) >= time.Minute {
				rateWindow, budget = event.observed, retirementLimit
			}
			if event.action == "remove" && budget > 0 && len(previous.byDevice[event.path]) > 0 {
				budget -= r.recordRemoval(ctx, previous, event, budget)
			}
			delete(previous.byDevice, event.path)
		}
	}
}

func (r *sensorRetirements) recordRemoval(ctx context.Context, previous sensorInventory, event hwmonEvent, limit int) int {
	ready := time.NewTimer(300 * time.Millisecond)
	defer ready.Stop()
	select {
	case <-ctx.Done():
		return 0
	case <-ready.C:
	}
	if !freshSensorRemoval(event.observed, time.Now()) {
		return 0
	}
	current, err := readInventory(ctx, r.root)
	if err != nil {
		return 0
	}
	removal := qualifyRemoval(r.root, previous, current, event, r.client, r.asset, limit)
	if count := len(removal.sensors); count > 0 {
		if err := r.publish(ctx, removal.events()); err != nil && len(r.pending) < retirementLimit {
			r.pending = append(r.pending, removal)
		}
		return count
	}
	return 0
}

func freshSensorRemoval(observed, now time.Time) bool {
	return !observed.After(now) && now.Sub(observed) < freshnessSeconds*time.Second
}

func qualifyRemoval(root string, previous, current sensorInventory, notice hwmonEvent, client, asset int64, limit int) sensorRemoval {
	removal := sensorRemoval{path: notice.path, observed: notice.observed}
	if !deviceAbsent(root, notice.path) {
		return removal
	}
	for _, key := range previous.byDevice[notice.path] {
		if len(removal.sensors) == limit {
			break
		}
		if previous.counts[key] != 1 || current.counts[key] != 0 {
			continue
		}
		event, err := monitoring.NewDiscoveryEvent(client, asset, "sensors", []string{key}, "tombstone", 100, freshnessSeconds, notice.observed)
		if err == nil {
			removal.sensors = append(removal.sensors, sensorCandidate{key: key, event: event})
		}
	}
	return removal
}

func deviceAbsent(root, path string) bool {
	_, err := os.Lstat(filepath.Join(root, path))
	return errors.Is(err, os.ErrNotExist)
}

func (r *sensorRetirements) retryPending(ctx context.Context, now time.Time) {
	r.pending = slices.DeleteFunc(r.pending, func(batch sensorRemoval) bool {
		return !freshSensorRemoval(batch.observed, now)
	})
	if len(r.pending) == 0 {
		return
	}
	inventory, err := readInventory(ctx, r.root)
	if err != nil || ctx.Err() != nil {
		return
	}
	r.pending = slices.DeleteFunc(r.pending, func(batch sensorRemoval) bool {
		return !deviceAbsent(r.root, batch.path)
	})
	for len(r.pending) > 0 {
		batch := &r.pending[0]
		batch.sensors = slices.DeleteFunc(batch.sensors, func(candidate sensorCandidate) bool {
			return inventory.counts[candidate.key] != 0
		})
		if len(batch.sensors) > 0 {
			if r.publish(ctx, batch.events()) == nil {
				r.pending = slices.Delete(r.pending, 0, 1)
			}
			return
		}
		r.pending = slices.Delete(r.pending, 0, 1)
	}
}

func (r sensorRemoval) events() []monitoring.DiscoveryEvent {
	events := make([]monitoring.DiscoveryEvent, 0, len(r.sensors))
	for _, candidate := range r.sensors {
		events = append(events, candidate.event)
	}
	return events
}
