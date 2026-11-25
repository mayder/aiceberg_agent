package packageretirement

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestDebianRetirementPreservesUpgradingAndResendsConfirmedRemoval(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	blocked := errors.New("outbox unavailable")
	var sent []monitoring.DiscoveryEvent
	tracker := packageRetirementTracker{source: "dpkg", client: 7, asset: 17, publish: func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		sent = append(sent, events...)
		if len(sent) == len(events) {
			return blocked
		}
		return nil
	}}
	for _, step := range []struct {
		pkg, want, state string
		err              error
		count            int
	}{
		{"fixture:amd64", "install", "installed", nil, 0},
		{"fixture:amd64", "install", "unpacked", nil, 0},
		{"fixture", "install", "installed", nil, 0},
		{"fixture", "deinstall", "config-files", blocked, 2},
		{"fixture", "install", "installed", nil, 2},
		{"fixture", "purge", "not-installed", nil, 4},
	} {
		rows, err := parseDebianInventory(step.pkg + "\tfixture\tamd64\t" + step.want + "\t" + step.state + "\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := tracker.observe(ctx, rows, now); !errors.Is(err, step.err) || len(sent) != step.count {
			t.Fatalf("state %s sent %+v: %v", step.state, sent, err)
		}
		now = now.Add(time.Second)
	}
	if sent[0].ObservedMS >= sent[2].ObservedMS || sent[2].Source != "applications" {
		t.Fatalf("reinstallation did not cancel the old removal: %+v", sent)
	}
}

func TestDebianPurgeRequiresRepeatedAbsence(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(1800000000, 0)
	initial, err := parseDebianInventory("fixture:arm64\tfixture\tarm64\tinstall\tinstalled\nanchor\tanchor\tall\tinstall\tinstalled\n")
	if err != nil {
		t.Fatal(err)
	}
	remaining, err := parseDebianInventory("anchor\tanchor\tall\tinstall\tinstalled\n")
	if err != nil {
		t.Fatal(err)
	}
	blocked := errors.New("outbox unavailable")
	var sent []monitoring.DiscoveryEvent
	tracker := packageRetirementTracker{source: "dpkg", client: 7, asset: 17, publish: func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		sent = append(sent, events...)
		if len(sent) == 1 {
			return blocked
		}
		return nil
	}}
	if err := tracker.observe(ctx, initial, now); err != nil {
		t.Fatal(err)
	}
	if err := tracker.observe(ctx, remaining, now); err != nil || len(sent) != 0 {
		t.Fatalf("single absence produced an event: %v, %+v", err, sent)
	}
	if err := tracker.observe(ctx, remaining, now.Add(time.Second)); !errors.Is(err, blocked) {
		t.Fatalf("confirmed absence did not reach outbox: %v", err)
	}
	if err := tracker.observe(ctx, remaining, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	first, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{"fixture:arm64", "", "dpkg"}, "tombstone", 100, 300, now.Add(time.Second))
	if err != nil || !slices.Equal(sent, []monitoring.DiscoveryEvent{first, first}) {
		t.Fatalf("confirmed retry was changed: %+v, %v", sent, err)
	}
}

func TestDebianInventoryRejectsMalformedAndAmbiguousRows(t *testing.T) {
	valid := "fixture\tfixture\tamd64\tinstall\tinstalled\n"
	for _, output := range []string{"", valid + "truncated\n", valid + valid, "bad:arm64\tbad\tamd64\tinstall\tinstalled\n"} {
		if _, err := parseDebianInventory(output); err == nil {
			t.Fatalf("incomplete package inventory accepted: %q", output)
		}
	}
}
