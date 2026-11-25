package serviceretirement

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const nativeServiceUnit = "[Unit]\nDescription=AIceberg service retirement fixture\n[Service]\nType=oneshot\nExecStart=/usr/bin/true\n"

func TestNativeSystemdServiceRemovalAndReturn(t *testing.T) {
	if os.Getenv("AICEBERG_DISCOVERY_SYSTEMD_FIXTURE") != "1" {
		t.Skip("requires an isolated systemd host")
	}
	if os.Geteuid() != 0 {
		t.Fatal("the isolated systemd fixture requires root to write its temporary unit")
	}
	unit, err := os.CreateTemp("/run/systemd/system", "aiceberg-service-fixture-*.service")
	if err != nil {
		t.Fatal(err)
	}
	path, name := unit.Name(), filepath.Base(unit.Name())
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
		reloadNativeServiceFixture(t)
	})
	writeNativeServiceFixture(t, unit)
	inventory, err := localSystemdInventory()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Unix(1795559000, 0).UTC()
	var events []monitoring.DiscoveryEvent
	outboxErr := errors.New("fixture outbox unavailable")
	publishErr := outboxErr
	tracker := retirementTracker{client: 7, asset: 17, inventory: inventory, caseSensitive: true,
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return publishErr
		}}
	if err := tracker.poll(ctx, now); err != nil || tracker.known[name] != name || len(events) != 0 {
		t.Fatalf("installed, inactive unit must remain discoverable: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reloadNativeServiceFixture(t)
	first := now.Add(time.Minute)
	if err := tracker.poll(ctx, first); !errors.Is(err, outboxErr) {
		t.Fatalf("expected the first retirement to reach the outbox: %v", err)
	}
	publishErr = nil
	if err := tracker.poll(ctx, first.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "services", []string{name}, "tombstone", 100, freshnessSeconds, first)
	if err != nil || len(events) != 2 || events[0] != want || events[1] != want {
		t.Fatalf("retry did not preserve the original service retirement: %v", err)
	}
	unit, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	writeNativeServiceFixture(t, unit)
	if err := tracker.poll(ctx, first.Add(time.Minute)); err != nil || tracker.known[name] != name || len(events) != 2 {
		t.Fatalf("restored service did not retain its identity: %v", err)
	}
}

func writeNativeServiceFixture(t *testing.T, unit *os.File) {
	t.Helper()
	_, writeErr := unit.WriteString(nativeServiceUnit)
	if err := errors.Join(writeErr, unit.Close()); err != nil {
		t.Fatal(err)
	}
	reloadNativeServiceFixture(t)
}

func reloadNativeServiceFixture(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "systemctl", "--system", "daemon-reload").CombinedOutput(); err != nil {
		t.Fatalf("reload isolated systemd fixture: %v: %s", err, out)
	}
}
