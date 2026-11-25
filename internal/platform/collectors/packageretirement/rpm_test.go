package packageretirement

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestRPMRetirementPreservesRemainingVersionsAndArchitectures(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	var events []monitoring.DiscoveryEvent
	tracker := packageRetirementTracker{source: "rpm", client: 7, asset: 17,
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return nil
		}}
	for i, output := range []string{
		"fixture\tx86_64\nfixture\tx86_64\nfixture\ti686\ngpg-pubkey\t(none)\n",
		"fixture\tx86_64\nfixture\ti686\n",
		"fixture\ti686\n",
		"fixture\ti686\n",
	} {
		inventory, err := parseRPMInventory(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := tracker.observe(ctx, inventory, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
		if i < 3 && len(events) != 0 {
			t.Fatalf("premature RPM retirement at capture %d: %+v", i, events)
		}
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{"fixture", "x86_64", "rpm"}, "tombstone", 100, 300, now.Add(3*time.Second))
	if err != nil || !slices.Equal(events, []monitoring.DiscoveryEvent{want}) {
		t.Fatalf("retirement crossed architectures: %+v, %v", events, err)
	}
}

func TestRPMInventoryRejectsIncompleteSnapshots(t *testing.T) {
	for _, output := range []string{"", "fixture\t(none)\n", "fixture\tx86_64\ntruncated\n", "gpg-pubkey\t(none)\n"} {
		if _, err := parseRPMInventory(output); err == nil {
			t.Fatalf("incomplete RPM output accepted: %q", output)
		}
	}
}
