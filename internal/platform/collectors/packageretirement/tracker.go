package packageretirement

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type packageRetirementTracker struct {
	client, asset int64
	source        string
	known         map[packageAddress]packageAddress
	absent        map[packageAddress]bool
	pending       map[packageAddress]monitoring.DiscoveryEvent
	publish       func(context.Context, []monitoring.DiscoveryEvent) error
}

func (r *packageRetirementTracker) observe(ctx context.Context, inventory map[packageAddress]packageRecord, now time.Time) error {
	if r.known == nil {
		r.known = map[packageAddress]packageAddress{}
		r.pending = map[packageAddress]monitoring.DiscoveryEvent{}
	}
	var confirmed []packageAddress
	var events []monitoring.DiscoveryEvent
	vanished := make(map[packageAddress]bool)
	currentNames := make(map[packageAddress]bool, len(inventory))
	ambiguousNames := make(map[packageAddress]bool)
	for address, pkg := range inventory {
		identity := r.identity(address, pkg)
		if pkg.state != "config-files" && pkg.state != "not-installed" {
			currentNames[identity] = true
		}
		if pkg.state == "ambiguous" {
			ambiguousNames[identity] = true
		}
	}
	for _, name := range slices.SortedFunc(maps.Keys(r.known), comparePackageAddresses) {
		if ambiguousNames[name] {
			delete(r.pending, name)
			delete(r.known, name)
			continue
		}
		address := r.known[name]
		pkg, present := inventory[address]
		if !present {
			vanished[address] = true
		}
		removed := present && (pkg.want == "deinstall" || pkg.want == "purge") && (pkg.state == "not-installed" || pkg.state == "config-files")
		if currentNames[name] || (present && !removed) {
			delete(r.pending, name)
			continue
		}
		if !present && !r.absent[address] {
			continue
		}
		if len(events) >= publicationLimit {
			continue
		}
		event, err := r.removal(name, now)
		if err != nil {
			return err
		}
		confirmed = append(confirmed, name)
		events = append(events, event)
	}
	r.absent = vanished
	for address, pkg := range inventory {
		identity := r.identity(address, pkg)
		_, known := r.known[identity]
		if pkg.state == "installed" && (known || len(r.known) < packageLimit) {
			r.known[identity] = address
		}
	}
	if len(events) == 0 {
		return nil
	}
	if err := r.publish(ctx, events); err != nil {
		return err
	}
	for _, name := range confirmed {
		delete(r.pending, name)
		delete(r.known, name)
	}
	return nil
}

func (r *packageRetirementTracker) identity(address packageAddress, pkg packageRecord) packageAddress {
	if r.source == "dpkg" || r.source == "registry" {
		return packageAddress{name: pkg.name}
	}
	return address
}

func comparePackageAddresses(left, right packageAddress) int {
	if cmp := strings.Compare(left.name, right.name); cmp != 0 {
		return cmp
	}
	return strings.Compare(left.architecture, right.architecture)
}

func (r *packageRetirementTracker) removal(name packageAddress, now time.Time) (monitoring.DiscoveryEvent, error) {
	if event, exists := r.pending[name]; exists && event.ObservedMS <= now.UnixMilli() && now.UnixMilli()-event.ObservedMS < freshnessSeconds*1000 {
		return event, nil
	}
	event, err := monitoring.NewDiscoveryEvent(r.client, r.asset, "applications", []string{name.name, name.architecture, r.source}, "tombstone", 100, freshnessSeconds, now)
	if err == nil {
		r.pending[name] = event
	}
	return event, err
}
