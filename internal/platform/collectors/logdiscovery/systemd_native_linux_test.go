package logdiscovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

const nativeSystemdFixtureUnit = "[Unit]\nDescription=AIceberg discovery test fixture\n[Service]\nType=oneshot\nExecStart=/usr/bin/true\n"

func TestNativeSystemdLogSourceRemovalAndReturn(t *testing.T) {
	if os.Getenv("AICEBERG_DISCOVERY_SYSTEMD_FIXTURE") != "1" {
		t.Skip("requires an isolated systemd host")
	}
	if os.Geteuid() != 0 {
		t.Fatal("the isolated systemd fixture requires root to write its temporary unit")
	}
	unit, err := os.CreateTemp("/run/systemd/system", "nginx-aiceberg-discovery-*.service")
	if err != nil {
		t.Fatal(err)
	}
	path := unit.Name()
	t.Cleanup(func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Error(err)
		}
		reloadFixtureSystemd(t)
	})
	_, writeErr := unit.WriteString(nativeSystemdFixtureUnit)
	if err := errors.Join(writeErr, unit.Close()); err != nil {
		t.Fatal(err)
	}
	reloadFixtureSystemd(t)

	collector := newFixtureCollector(config.Config{LogDiscoveryEnabled: true, LogDiscoveryMaxCandidates: 1000}, nil, nil)
	watcher := NewSourceRetirement(7, 17)
	now := time.Unix(1795559000, 0)
	var events []monitoring.DiscoveryEvent
	publish := func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
		events = append(events, batch...)
		return nil
	}
	original := observeNativeSystemdFixture(t, collector, watcher, path, now, publish)
	if original == "" || len(events) != 0 {
		t.Fatal("installed, inactive unit must be discovered and preserved")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	reloadFixtureSystemd(t)
	first := now.Add(time.Minute)
	if got := observeNativeSystemdFixture(t, collector, watcher, path, first, publish); got != "" || len(events) != 0 {
		t.Fatal("first confirmation did not preserve retirement grace")
	}
	confirmed := first.Add(retirementGrace)
	observeNativeSystemdFixture(t, collector, watcher, path, confirmed, publish)
	want, err := monitoring.NewDiscoveryEvent(7, 17, "log_sources", []string{original}, "tombstone", 100, retirementFreshness, confirmed)
	if err != nil || len(events) != 1 || events[0] != want {
		t.Fatalf("expected one retirement for the missing systemd unit, got %v: %v", events, err)
	}
	restoreFixtureSystemdUnit(t, path)
	if got := observeNativeSystemdFixture(t, collector, watcher, path, confirmed.Add(time.Minute), publish); got != original || len(events) != 1 {
		t.Fatal("restored systemd unit did not retain its former discovery identity")
	}
}

func observeNativeSystemdFixture(t *testing.T, collector *Collector, watcher *SourceRetirement, path string,
	at time.Time, publish func(context.Context, []monitoring.DiscoveryEvent) error,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	raw, err := collector.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := watcher.ObserveBuffered(ctx, raw, at, publish); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]payload
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range snapshot[schemaVersion].Candidates {
		if candidate.Kind == "systemd_unit" && candidate.Path == path {
			return candidate.Fingerprint
		}
	}
	return ""
}

func restoreFixtureSystemdUnit(t *testing.T, path string) {
	t.Helper()
	unit, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := unit.WriteString(nativeSystemdFixtureUnit)
	if err := errors.Join(writeErr, unit.Close()); err != nil {
		t.Fatal(err)
	}
	reloadFixtureSystemd(t)
}

func reloadFixtureSystemd(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "systemctl", "--system", "daemon-reload").CombinedOutput(); err != nil {
		t.Fatalf("reload isolated systemd fixture: %v: %s", err, output)
	}
}
