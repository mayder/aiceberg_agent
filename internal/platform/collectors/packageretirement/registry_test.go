package packageretirement

import (
	"context"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestRegistryApplicationRetirementKeepsInventoryIdentityAcrossViews(t *testing.T) {
	tracker, events := registryTestTracker()
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, time.UTC)
	first := registryApplication{"64", "{first}", " Fixture App "}
	moved := registryApplication{"32", "{second}", "Fixture App"}
	observeRegistryApplications(t, tracker, now, first)
	observeRegistryApplications(t, tracker, now.Add(time.Minute), moved)
	observeRegistryApplications(t, tracker, now.Add(2*time.Minute))
	last := now.Add(3 * time.Minute)
	if len(*events) != 0 {
		t.Fatal("moving the registration and the first absent capture must keep the application")
	}
	observeRegistryApplications(t, tracker, last)
	want, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{"Fixture App", "", "registry"}, "tombstone", 100, freshnessSeconds, last)
	if err != nil {
		t.Fatal(err)
	}
	if len(*events) != 1 || (*events)[0] != want {
		t.Fatalf("got %+v, want %+v", *events, want)
	}
}

func TestRegistryApplicationRetirementProtectsAmbiguousAndRenamedEntries(t *testing.T) {
	now := time.Date(2026, 9, 22, 18, 0, 0, 0, time.UTC)
	first := registryApplication{"64", "{first}", "Fixture App"}
	duplicate := registryApplication{"32", "{second}", "Fixture App"}
	for _, tc := range []struct {
		name     string
		captures [][]registryApplication
	}{
		{"duplicate across views", [][]registryApplication{{first}, {first, duplicate}, nil, nil}},
		{"renamed in place", [][]registryApplication{{first}, {{"64", "{FIRST}", "Renamed App"}}, {{"64", "{first}", "Renamed App"}}}},
		{"title unreadable", [][]registryApplication{{first}, {{"64", "{first}", ""}}, {{"64", "{first}", ""}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracker, events := registryTestTracker()
			for i, applications := range tc.captures {
				observeRegistryApplications(t, tracker, now.Add(time.Duration(i)*time.Minute), applications...)
			}
			if len(*events) != 0 {
				t.Fatalf("unexpected retirement: %+v", *events)
			}
		})
	}
}

func TestRegistryApplicationInventoryRejectsRepeatedAddresses(t *testing.T) {
	_, err := normalizeRegistryApplications([]registryApplication{
		{"32", "{KEY}", "Fixture App"},
		{"32", "{key}", "Other App"},
	})
	if err == nil {
		t.Fatal("a repeated key in the same view must invalidate the capture")
	}
}

func registryTestTracker() (*packageRetirementTracker, *[]monitoring.DiscoveryEvent) {
	var events []monitoring.DiscoveryEvent
	tracker := &packageRetirementTracker{
		client: 7, asset: 17, source: "registry",
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return nil
		},
	}
	return tracker, &events
}

func observeRegistryApplications(t *testing.T, tracker *packageRetirementTracker, now time.Time, applications ...registryApplication) {
	t.Helper()
	inventory, err := normalizeRegistryApplications(applications)
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.observe(context.Background(), inventory, now); err != nil {
		t.Fatal(err)
	}
}
