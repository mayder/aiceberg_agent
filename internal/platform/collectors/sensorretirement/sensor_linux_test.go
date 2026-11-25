package sensorretirement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestHwmonRetirementRequiresRemovedUniqueTemperature(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOST_SYS", root)
	t.Setenv("HOST_ROOT", "")
	class := filepath.Join(root, "class", "hwmon")
	if err := os.MkdirAll(class, 0755); err != nil {
		t.Fatal(err)
	}
	path := "/devices/platform/coretemp.0/hwmon/hwmon0"
	chip := addTestHwmon(t, root, path, "Core 0")
	notice := hwmonEvent{action: "remove", path: path, observed: time.Unix(12345, 0)}
	previous := readTestInventory(t, root)
	if previous.counts["coretemp_core_0"] != 1 {
		t.Fatalf("collector provenance = %+v", previous)
	}
	assertBatchSize := func(previous, current sensorInventory, size int) {
		t.Helper()
		batch := qualifyRemoval(root, previous, current, notice, 7, 8, retirementLimit).events()
		if len(batch) != size {
			t.Fatalf("batch = %+v, want %d entries", batch, size)
		}
		if size == 1 {
			want, err := monitoring.NewDiscoveryEvent(7, 8, "sensors", []string{"coretemp_core_0"}, "tombstone", 100, 300, notice.observed)
			if err != nil || batch[0] != want {
				t.Fatalf("event = %+v, want %+v (%v)", batch[0], want, err)
			}
		}
	}
	t.Run("device still exists", func(t *testing.T) {
		assertBatchSize(previous, previous, 0)
	})
	other := "/devices/platform/coretemp.1/hwmon/hwmon1"
	otherChip := addTestHwmon(t, root, other, "Core 0")
	duplicate := readTestInventory(t, root)
	removeTestHwmon(t, class, chip)
	withDuplicateStillPresent := readTestInventory(t, root)
	t.Run("identity still reported by another device", func(t *testing.T) {
		assertBatchSize(previous, withDuplicateStillPresent, 0)
	})
	removeTestHwmon(t, class, otherChip)
	absent := readTestInventory(t, root)
	t.Run("prior identity shared by two devices", func(t *testing.T) {
		assertBatchSize(duplicate, absent, 0)
	})
	t.Run("known unique identity removed", func(t *testing.T) {
		assertBatchSize(previous, absent, 1)
	})
	t.Run("thermal fallback still reports same key", func(t *testing.T) {
		zone := filepath.Join(root, "class", "thermal", "thermal_zone0")
		if err := os.MkdirAll(zone, 0755); err != nil {
			t.Fatal(err)
		}
		writeTestFiles(t, zone, map[string]string{"type": "coretemp_core_0\n", "temp": "44000\n"})
		fallback := readTestInventory(t, root)
		assertBatchSize(previous, fallback, 0)
		if len(fallback.byDevice) != 0 {
			t.Fatalf("fallback was attributed to a hwmon device: %+v", fallback)
		}
	})
}

func TestHwmonRetirementRetriesOnlyWhileTheOriginalRemovalIsValid(t *testing.T) {
	for _, scenario := range []string{"absent", "inventory unavailable", "reappeared under another path", "expired"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOST_SYS", root)
			t.Setenv("HOST_ROOT", "")
			class := filepath.Join(root, "class", "hwmon")
			if err := os.MkdirAll(class, 0755); err != nil {
				t.Fatal(err)
			}
			notice := hwmonEvent{"remove", "/devices/platform/coretemp.0/hwmon/hwmon0", time.Now()}
			chip := addTestHwmon(t, root, notice.path, "Core 0")
			previous := readTestInventory(t, root)
			removeTestHwmon(t, class, chip)
			var attempts [][]monitoring.DiscoveryEvent
			retirements := sensorRetirements{root: root, client: 7, asset: 8, publish: func(_ context.Context, events []monitoring.DiscoveryEvent) error {
				attempts = append(attempts, events)
				if len(attempts) == 1 {
					return errors.New("fixture outbox unavailable")
				}
				return nil
			}}
			ctx := context.Background()
			if count := retirements.recordRemoval(ctx, previous, notice, retirementLimit); count != 1 || len(retirements.pending) != 1 {
				t.Fatalf("first attempt admitted %d events, staged %d retries", count, len(retirements.pending))
			}
			now, want := notice.observed.Add(time.Second), 1
			switch scenario {
			case "absent":
				want = 2
			case "inventory unavailable":
				if err := os.Rename(class, class+"-unavailable"); err != nil {
					t.Fatal(err)
				}
				retirements.retryPending(ctx, now)
				if len(attempts) != 1 || len(retirements.pending) != 1 {
					t.Fatal("unavailable inventory must defer the retry")
				}
				if err := os.Rename(class+"-unavailable", class); err != nil {
					t.Fatal(err)
				}
				want = 2
			case "reappeared under another path":
				addTestHwmon(t, root, "/devices/platform/coretemp.1/hwmon/hwmon1", "Core 0")
			case "expired":
				now = notice.observed.Add(freshnessSeconds * time.Second)
			}
			retirements.retryPending(ctx, now)
			if len(retirements.pending) != 0 || len(attempts) != want {
				t.Fatalf("retry left %d pending entries and %d attempts; want %d attempts", len(retirements.pending), len(attempts), want)
			}
			if want == 2 && (len(attempts[1]) != 1 || attempts[0][0] != attempts[1][0]) {
				t.Fatalf("retry changed the original event: %+v", attempts)
			}
		})
	}
}

func TestHwmonWatchContinuesAfterOutboxFailure(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOST_SYS", root)
	t.Setenv("HOST_ROOT", "")
	class := filepath.Join(root, "class", "hwmon")
	if err := os.MkdirAll(class, 0755); err != nil {
		t.Fatal(err)
	}
	paths := []string{"/devices/platform/coretemp.0/hwmon/hwmon0", "/devices/platform/coretemp.1/hwmon/hwmon1"}
	first := addTestHwmon(t, root, paths[0], "Core 0")
	second := addTestHwmon(t, root, paths[1], "Core 1")
	previous := readTestInventory(t, root)
	removeTestHwmon(t, class, first)
	removeTestHwmon(t, class, second)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events := make(chan hwmonEvent, len(paths))
	for _, path := range paths {
		events <- hwmonEvent{"remove", path, time.Now()}
	}
	var delivered string
	attempts := 0
	retirements := sensorRetirements{root: root, client: 7, asset: 8, publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
		attempts++
		if attempts == 1 {
			return errors.New("fixture outbox unavailable")
		}
		delivered = batch[0].Fingerprint
		cancel()
		return nil
	}}
	if err := retirements.watch(ctx, previous, events, nil); err != nil {
		t.Fatal(err)
	}
	expected, err := monitoring.NewDiscoveryEvent(7, 8, "sensors", []string{"coretemp_core_1"}, "tombstone", 100, freshnessSeconds, time.Now())
	if err != nil || attempts != 2 || delivered != expected.Fingerprint || len(retirements.pending) != 1 {
		t.Fatalf("listener attempts=%d delivered=%q pending=%d err=%v", attempts, delivered, len(retirements.pending), err)
	}
}

func addTestHwmon(t *testing.T, root, path, label string) string {
	t.Helper()
	target := filepath.Join(root, path)
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFiles(t, target, map[string]string{"name": "coretemp\n", "temp1_label": label + "\n", "temp1_input": "43000\n"})
	if err := os.Symlink(target, filepath.Join(root, "class", "hwmon", filepath.Base(target))); err != nil {
		t.Fatal(err)
	}
	return target
}

func removeTestHwmon(t *testing.T, class, chip string) {
	t.Helper()
	if err := os.Remove(filepath.Join(class, filepath.Base(chip))); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(chip); err != nil {
		t.Fatal(err)
	}
}

func writeTestFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func readTestInventory(t *testing.T, root string) sensorInventory {
	t.Helper()
	result, err := readInventory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
