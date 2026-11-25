package serviceretirement

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const (
	serviceLimit     = 1024
	batchLimit       = 16
	probeLimit       = 64
	freshnessSeconds = 300
)

type serviceInventory interface {
	list(context.Context) ([]string, error)
	missing(context.Context, string) (bool, error)
}

type retirementTracker struct {
	client, asset int64
	inventory     serviceInventory
	publish       func(context.Context, []monitoring.DiscoveryEvent) error
	known         map[string]string
	pending       map[string]monitoring.DiscoveryEvent
	cursor        string
	caseSensitive bool
}

func (r *retirementTracker) poll(ctx context.Context, now time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	listed, err := r.inventory.list(ctx)
	if err != nil || len(listed) > serviceLimit {
		return nil
	}
	current := serviceNames(listed, r.caseSensitive)
	if r.known == nil {
		r.known = map[string]string{}
		r.pending = map[string]monitoring.DiscoveryEvent{}
	}
	for key, name := range current {
		if _, exists := r.known[key]; exists || len(r.known) < serviceLimit {
			r.known[key] = name
		}
		delete(r.pending, key)
	}
	return r.confirmRemovals(ctx, now, current)
}

func serviceNames(listed []string, caseSensitive bool) map[string]string {
	names := make(map[string]string, len(listed))
	for _, name := range listed {
		if name != "" && strings.TrimSpace(name) == name && utf8.RuneCountInString(name) <= 255 &&
			utf8.ValidString(name) && !strings.ContainsAny(name, "\x00\r\n\v") {
			key := name
			if !caseSensitive {
				key = strings.ToLower(name)
			}
			names[key] = name
		}
	}
	return names
}

func (r *retirementTracker) confirmRemovals(ctx context.Context, now time.Time, current map[string]string) error {
	keys := slices.Sorted(maps.Keys(r.known))
	start, found := slices.BinarySearch(keys, r.cursor)
	if found {
		start++
	}
	var events []monitoring.DiscoveryEvent
	var accepted []string
	probed := 0
	for offset := range keys {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := keys[(start+offset)%len(keys)]
		r.cursor = key
		if _, exists := current[key]; exists {
			continue
		}
		probed++
		if event, confirmed := r.confirmRemoval(ctx, key, now); confirmed {
			events = append(events, event)
			accepted = append(accepted, key)
		}
		if len(events) == batchLimit || probed == probeLimit {
			break
		}
	}
	if len(events) == 0 {
		return nil
	}
	if err := r.publish(ctx, events); err != nil {
		return err
	}
	for _, key := range accepted {
		delete(r.known, key)
		delete(r.pending, key)
	}
	return nil
}

func (r *retirementTracker) confirmRemoval(ctx context.Context, key string, now time.Time) (monitoring.DiscoveryEvent, bool) {
	missing, err := r.inventory.missing(ctx, r.known[key])
	if err != nil {
		return monitoring.DiscoveryEvent{}, false
	}
	if !missing {
		delete(r.pending, key)
		return monitoring.DiscoveryEvent{}, false
	}
	if event, exists := r.pending[key]; exists && event.ObservedMS <= now.UnixMilli() && now.UnixMilli()-event.ObservedMS < freshnessSeconds*1000 {
		return event, true
	}
	event, err := monitoring.NewDiscoveryEvent(r.client, r.asset, "services", []string{r.known[key]}, "tombstone", 100, freshnessSeconds, now)
	if err != nil {
		return monitoring.DiscoveryEvent{}, false
	}
	r.pending[key] = event
	return event, true
}
