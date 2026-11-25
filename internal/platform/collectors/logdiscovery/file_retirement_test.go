package logdiscovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestFileRetirementConfirmsAbsenceAndRetriesSameEvent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "error.log")
	putFixtureLog(t, path)
	collector, watcher := newFileRetirementFixture(path)
	ctx := context.Background()
	now := time.Unix(1795550000, 0)
	var attempts []monitoring.DiscoveryEvent
	unavailable := errors.New("fixture outbox unavailable")
	publish := func(_ context.Context, events []monitoring.DiscoveryEvent) error {
		attempts = append(attempts, events...)
		if len(attempts) == 1 {
			return unavailable
		}
		return nil
	}
	observe := func(at time.Time) error {
		raw, err := collector.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return watcher.ObserveBuffered(ctx, raw, at, publish)
	}
	if err := observe(now); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{now.Add(time.Minute), now.Add(2 * time.Minute)} {
		if err := observe(at); err != nil || len(attempts) != 0 {
			t.Fatalf("rotation grace emitted early: %v, %v", attempts, err)
		}
	}
	at := now.Add(3 * time.Minute)
	if err := observe(at); !errors.Is(err, unavailable) {
		t.Fatalf("outbox failure not returned: %v", err)
	}
	if err := observe(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	row := collector.knownPathCandidate(collector.knownPaths[0], path, fixtureRotatedFile(t, path))
	want, err := monitoring.NewDiscoveryEvent(7, 17, "log_sources", []string{row.Fingerprint}, "tombstone", 100, retirementFreshness, at)
	if err != nil || len(attempts) != 2 || attempts[0] != want || attempts[1] != want {
		t.Fatalf("expected identical retries of %v, got %v (%v)", want, attempts, err)
	}
	if err := observe(at.Add(2 * time.Minute)); err != nil || len(attempts) != 2 {
		t.Fatalf("buffered retirement was repeated: %v, %v", attempts, err)
	}
}

func TestFileRetirementPreservesRotationAndUnavailableDirectory(t *testing.T) {
	for _, change := range []string{"file returned", "directory replaced", "discovery disabled"} {
		t.Run(change, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "logs")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "error.log")
			putFixtureLog(t, path)
			collector, watcher := newFileRetirementFixture(path)
			ctx := context.Background()
			now := time.Unix(1795550000, 0)
			publish := func(context.Context, []monitoring.DiscoveryEvent) error {
				t.Fatal("temporary absence must never emit a retirement")
				return nil
			}
			observe := func(at time.Time) {
				raw, err := collector.Collect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := watcher.ObserveBuffered(ctx, raw, at, publish); err != nil {
					t.Fatal(err)
				}
			}
			observe(now)
			if err := os.Rename(path, path+".1"); err != nil {
				t.Fatal(err)
			}
			observe(now.Add(time.Minute))
			switch change {
			case "file returned":
				putFixtureLog(t, path)
			case "directory replaced":
				if err := os.Rename(dir, dir+".moved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			case "discovery disabled":
				collector.cfg.LogDiscoveryEnabled = false
			}
			observe(now.Add(3 * time.Minute))
			collector.cfg.LogDiscoveryEnabled = true
			observe(now.Add(6 * time.Minute))
		})
	}
}

func newFileRetirementFixture(path string) (*Collector, *SourceRetirement) {
	known := []knownPath{{Path: path, Kind: "log_file", Product: "nginx", ServiceName: "nginx"}}
	return newFixtureCollector(config.Config{LogDiscoveryEnabled: true}, nil, known), NewSourceRetirement(7, 17)
}

func putFixtureLog(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("fixture log\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixtureRotatedFile(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	return info
}
