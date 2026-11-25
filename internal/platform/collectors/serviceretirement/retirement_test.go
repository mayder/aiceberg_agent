package serviceretirement

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type fixtureServiceInventory struct {
	listed   []string
	removed  map[string]bool
	listErr  error
	probeErr error
}

func (s *fixtureServiceInventory) list(context.Context) ([]string, error) { return s.listed, s.listErr }
func (s *fixtureServiceInventory) missing(_ context.Context, name string) (bool, error) {
	return s.removed[name], s.probeErr
}

func TestServiceRetirementRequiresAuthoritativeAbsence(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	inventory := &fixtureServiceInventory{listed: []string{"StillInstalled", "Removed", "returns"}}
	var events []monitoring.DiscoveryEvent
	tracker := retirementTracker{client: 7, asset: 17, inventory: inventory, publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
		events = append(events, batch...)
		return nil
	}}
	if err := tracker.poll(ctx, now); err != nil {
		t.Fatal(err)
	}
	inventory.listed = nil
	inventory.removed = map[string]bool{"Removed": true, "NeverSeen": true}
	inventory.listErr = errors.New("enumeration failed")
	if err := tracker.poll(ctx, now); err != nil || len(events) != 0 {
		t.Fatalf("incomplete enumeration produced a removal: %v, %+v", err, events)
	}
	inventory.listErr, inventory.probeErr = nil, errors.New("access denied")
	if err := tracker.poll(ctx, now); err != nil || len(events) != 0 {
		t.Fatalf("inconclusive probe produced a removal: %v, %+v", err, events)
	}
	inventory.probeErr = nil
	inventory.listed = []string{"RETURNS"}
	if err := tracker.poll(ctx, now); err != nil {
		t.Fatal(err)
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "services", []string{"Removed"}, "tombstone", 100, 300, now)
	if err != nil || !slices.Equal(events, []monitoring.DiscoveryEvent{want}) {
		t.Fatalf("events=%+v, want=%+v: %v", events, want, err)
	}
	if err := tracker.poll(ctx, now.Add(time.Second)); err != nil || len(events) != 1 {
		t.Fatalf("removal duplicated on polling: %v, %+v", err, events)
	}
}

func TestServiceRetirementRetriesAndRechecksPresence(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	inventory := &fixtureServiceInventory{listed: []string{"Gone", "Restored"}}
	outage := errors.New("outbox unavailable")
	var first, retry []monitoring.DiscoveryEvent
	tracker := retirementTracker{client: 7, asset: 17, inventory: inventory, publish: func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		if first == nil {
			first = slices.Clone(events)
			return outage
		}
		retry = slices.Clone(events)
		return nil
	}}
	if err := tracker.poll(ctx, now); err != nil {
		t.Fatal(err)
	}
	inventory.listed = nil
	inventory.removed = map[string]bool{"Gone": true, "Restored": true}
	if err := tracker.poll(ctx, now); !errors.Is(err, outage) || len(first) != 2 {
		t.Fatalf("expected two pending removals: %v, %+v", err, first)
	}
	inventory.listed = []string{"Restored"}
	if err := tracker.poll(ctx, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(retry, first[:1]) {
		t.Fatalf("retry was rewritten or included a restored service: %+v vs %+v", retry, first)
	}
}
