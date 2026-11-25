package serviceretirement

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestSystemdRetirementTracksInstalledServices(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	listed := `Demo.service enabled enabled
demo.service disabled disabled
stopped.service static -
masked.service masked enabled
active.service enabled enabled
fixture@.service disabled disabled
dynamic.service transient -
generated.service generated -
`
	var calls []string
	inventory := systemdServiceInventory{run: func(_ context.Context, args ...string) (string, int, error) {
		if args[0] == "list-unit-files" {
			return listed, 0, nil
		}
		if len(args) != 4 || args[0] != "show" || args[2] != "--" {
			t.Fatalf("unexpected systemd query: %v", args)
		}
		name := args[3]
		calls = append(calls, name)
		switch name {
		case "Demo.service":
			return "ActiveState=inactive\nLoadState=not-found\n", 4, nil
		case "stopped.service":
			return "LoadState=loaded\nActiveState=inactive\n", 0, nil
		case "masked.service":
			return "LoadState=masked\nActiveState=inactive\n", 0, nil
		case "active.service":
			return "LoadState=not-found\nActiveState=active\n", 0, nil
		default:
			t.Fatalf("unexpected probe of %s", name)
			return "", 1, nil
		}
	}}
	var events []monitoring.DiscoveryEvent
	tracker := retirementTracker{client: 7, asset: 17, inventory: inventory, caseSensitive: true,
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return nil
		}}
	if err := tracker.poll(ctx, now); err != nil {
		t.Fatal(err)
	}
	listed = "demo.service disabled disabled\n"
	if err := tracker.poll(ctx, now); err != nil {
		t.Fatal(err)
	}
	slices.Sort(calls)
	if !slices.Equal(calls, []string{"Demo.service", "active.service", "masked.service", "stopped.service"}) {
		t.Fatalf("unexpected service lookups: %v", calls)
	}
	wanted, err := monitoring.NewDiscoveryEvent(7, 17, "services", []string{"Demo.service"}, "tombstone", 100, 300, now)
	if err != nil || !slices.Equal(events, []monitoring.DiscoveryEvent{wanted}) {
		t.Fatalf("events=%+v, want=%+v: %v", events, wanted, err)
	}
}

func TestSystemdRetirementRejectsInconclusiveQueries(t *testing.T) {
	for _, test := range []struct {
		output string
		code   int
		err    error
	}{
		{"LoadState=not-found\nActiveState=inactive", 1, nil},
		{"LoadState=not-found\nActiveState=inactive", 0, errors.New("incomplete output")},
		{"LoadState=not-found\nActiveState=inactive\nLoadState=loaded", 0, nil},
		{"LoadState=not-found", 0, nil},
		{"", 4, nil},
	} {
		inventory := systemdServiceInventory{run: func(context.Context, ...string) (string, int, error) {
			return test.output, test.code, test.err
		}}
		if missing, _ := inventory.missing(context.Background(), "fixture.service"); missing {
			t.Fatalf("inconclusive response admitted: %+v", test)
		}
	}
	inventory := systemdServiceInventory{run: func(context.Context, ...string) (string, int, error) {
		return "unit.service disabled\nmalformed-output\n", 0, nil
	}}
	if _, err := inventory.list(context.Background()); err == nil {
		t.Fatal("partial systemd output accepted as authoritative inventory")
	}
}
