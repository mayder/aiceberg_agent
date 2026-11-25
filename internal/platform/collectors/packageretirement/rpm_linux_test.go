package packageretirement

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestRPMRetirementUsesNativeDatabaseInDisposableRoot(t *testing.T) {
	if os.Getenv("AICEBERG_TEST_DISPOSABLE_RPM") != "1" {
		t.Skip("requires disposable RPM environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	path, err := exec.LookPath("rpm")
	if err != nil {
		t.Fatal(err)
	}
	output, err := readPackageOutput(exec.CommandContext(ctx, path, "--eval", "%{_dbpath}"))
	if err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSpace(output)
	if source != "/var/lib/rpm" && source != "/usr/lib/sysimage/rpm" {
		t.Fatalf("unexpected RPM source database: %q", source)
	}
	dir := t.TempDir()
	if output, err := exec.CommandContext(ctx, "cp", "-a", source+"/.", dir).CombinedOutput(); err != nil {
		t.Fatalf("copy fixture RPM database: %v\n%s", err, output)
	}
	before, err := readRPMInventory(ctx, path, dir)
	if err != nil {
		t.Fatal(err)
	}
	var removed []packageAddress
	for address := range before {
		if address.name == "bash" {
			removed = append(removed, address)
		}
	}
	if len(removed) != 1 {
		t.Fatalf("expected one bash architecture in the RPM fixture: %+v", removed)
	}
	var events []monitoring.DiscoveryEvent
	tracker := packageRetirementTracker{source: "rpm", client: 7, asset: 17,
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return nil
		}}
	now := time.Unix(1800000000, 0)
	if err := tracker.observe(ctx, before, now); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, path, "--dbpath", dir, "-e", "--allmatches", "--justdb", "--nodeps",
		"--noscripts", "--notriggers", "bash").CombinedOutput(); err != nil {
		t.Fatalf("erase entry from copied RPM database: %v\n%s", err, output)
	}
	for i := 1; i <= 2; i++ {
		inventory, err := readRPMInventory(ctx, path, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := tracker.observe(ctx, inventory, now.Add(time.Duration(i)*time.Second)); err != nil || len(events) != i-1 {
			t.Fatalf("RPM removal at capture %d: %+v, %v", i, events, err)
		}
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{"bash", removed[0].architecture, "rpm"}, "tombstone", 100, 300, now.Add(2*time.Second))
	if err != nil || !slices.Equal(events, []monitoring.DiscoveryEvent{want}) {
		t.Fatalf("native RPM retirement events: %+v, %v", events, err)
	}
}
