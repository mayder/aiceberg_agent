package packageretirement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/windows/registry"
)

func TestNativeRegistryApplicationRemovalAndReturn(t *testing.T) {
	if os.Getenv("AICEBERG_DISCOVERY_WINDOWS_REGISTRY_FIXTURE") != "1" {
		t.Skip("requires an isolated Windows host and permission to write the HKLM uninstall registry")
	}
	views, err := applicationRegistryViews()
	if err != nil {
		t.Fatal(err)
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, uninstallKey, registry.READ|registry.CREATE_SUB_KEY)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	keyName := "AIceberg-Registry-Fixture-" + hex.EncodeToString(random[:])
	title := "AIceberg Registry Fixture " + hex.EncodeToString(random[:])
	createNativeApplicationFixture(t, root, keyName, title)
	t.Cleanup(func() {
		if err := registry.DeleteKey(root, keyName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			t.Error(err)
		}
	})

	tracker, events := registryTestTracker()
	now := time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	observe := func(at time.Time) {
		t.Helper()
		inventory, err := readRegistryApplications(ctx, views)
		if err != nil {
			t.Fatal(err)
		}
		// Real inventory can contain other installed software. The fixture
		// asserts only its own unique identity and never touches other keys.
		for address, record := range inventory {
			if record.name != title {
				delete(inventory, address)
			}
		}
		if err := tracker.observe(ctx, inventory, at); err != nil {
			t.Fatal(err)
		}
	}
	observe(now)
	if len(tracker.known) != 1 || len(*events) != 0 {
		t.Fatal("native Registry capture did not track the fixture")
	}
	if err := registry.DeleteKey(root, keyName); err != nil {
		t.Fatal(err)
	}
	observe(now.Add(time.Minute))
	if len(*events) != 0 {
		t.Fatal("one absent capture must not retire the Registry application")
	}
	at := now.Add(2 * time.Minute)
	observe(at)
	want, err := monitoring.NewDiscoveryEvent(7, 17, "applications", []string{title, "", "registry"},
		"tombstone", 100, freshnessSeconds, at)
	if err != nil || len(*events) != 1 || (*events)[0] != want {
		t.Fatalf("native Registry removal did not match the inventory identity: %v", err)
	}
	createNativeApplicationFixture(t, root, keyName, title)
	observe(now.Add(3 * time.Minute))
	if len(tracker.known) != 1 || len(*events) != 1 {
		t.Fatal("restored registry key did not recover the application identity")
	}
}

func createNativeApplicationFixture(t *testing.T, root registry.Key, name, title string) {
	t.Helper()
	key, existed, err := registry.CreateKey(root, name, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	if existed {
		t.Fatal("refusing to modify a Registry key that already exists")
	}
	if err := key.SetStringValue("DisplayName", title); err != nil {
		_ = key.Close()
		_ = registry.DeleteKey(root, name)
		t.Fatal(err)
	}
}
