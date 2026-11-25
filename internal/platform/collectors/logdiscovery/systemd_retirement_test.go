package logdiscovery

import (
	"context"
	"encoding/json"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestSystemdLogSourceWaitsForUnitAndPathToBeAbsent(t *testing.T) {
	dir := t.TempDir()
	unit := "nginx-fixture.service"
	path := filepath.Join(dir, unit)
	putFixtureLog(t, path)
	watcher := NewSourceRetirement(7, 17)
	ctx := context.Background()
	candidate := Candidate{Kind: "systemd_unit", Path: path, Unit: unit, Runtime: "systemd"}
	candidate.Fingerprint = fingerprint(candidate)
	now := time.Unix(1795550000, 0)
	var events []monitoring.DiscoveryEvent
	publish := func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
		events = append(events, batch...)
		return nil
	}
	observe := func(at time.Time, rows ...Candidate) {
		t.Helper()
		raw, err := json.Marshal(map[string]payload{schemaVersion: {Candidates: rows}})
		if err == nil {
			err = watcher.ObserveBuffered(ctx, raw, at, publish)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	var missing bool
	var probeErr error
	watcher.probeSystemd = func(_ context.Context, name string) (bool, error) {
		if name != unit {
			t.Fatalf("unexpected service %s", name)
		}
		return missing, probeErr
	}
	observe(now, candidate)
	if err := os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	observe(now.Add(time.Minute))
	missing = true
	observe(now.Add(2 * time.Minute))
	probeErr = fs.ErrPermission
	observe(now.Add(4 * time.Minute))
	probeErr = nil
	observe(now.Add(5 * time.Minute))
	if len(events) != 0 {
		t.Fatalf("unconfirmed unit retirement: %+v", events)
	}
	at := now.Add(7 * time.Minute)
	observe(at)
	want, err := monitoring.NewDiscoveryEvent(7, 17, "log_sources", []string{candidate.Fingerprint}, "tombstone", 100, retirementFreshness, at)
	if err != nil || len(events) != 1 || events[0] != want {
		t.Fatalf("expected unit retirement %v, got %v (%v)", want, events, err)
	}
}

func TestJournaldRetirementPreservesInstalledServiceWithMissingSocket(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Unix domain socket fixture")
	}
	dir, err := os.MkdirTemp("/tmp", "aiceberg-journal-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(dir); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(dir, "journal.sock")
	socket, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	watcher := NewSourceRetirement(7, 17)
	missing := false
	watcher.probeSystemd = func(_ context.Context, unit string) (bool, error) {
		if unit != "systemd-journald.service" {
			t.Fatalf("unexpected service %s", unit)
		}
		return missing, nil
	}
	absent := watcher.systemdSourceAbsenceProbe(path, "systemd-journald.service", fs.ModeSocket)
	if absent == nil {
		t.Fatal("valid journal socket was not tracked")
	}
	if err := socket.Close(); err != nil {
		t.Fatal(err)
	}
	if absent(context.Background()) {
		t.Fatal("socket disappearance alone cannot retire journald")
	}
	missing = true
	if !absent(context.Background()) {
		t.Fatal("both socket and service are absent")
	}
}
