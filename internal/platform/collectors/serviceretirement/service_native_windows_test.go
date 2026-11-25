package serviceretirement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

func TestNativeWindowsServiceRemovalAndReturn(t *testing.T) {
	if os.Getenv("AICEBERG_DISCOVERY_WINDOWS_SERVICE_FIXTURE") != "1" {
		t.Skip("requires an isolated Windows host with access to create a disabled test service")
	}
	root := os.Getenv("SystemRoot")
	if !filepath.IsAbs(root) {
		t.Fatal("SystemRoot is unavailable")
	}
	manager, err := mgr.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Disconnect() })
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := "AIceberg-Service-Fixture-" + hex.EncodeToString(random[:])
	binary := filepath.Join(root, "System32", "cmd.exe")
	createSCMFixture(t, manager, name, binary)
	t.Cleanup(func() {
		if err := deleteSCMFixture(manager, name); err != nil && !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			t.Error(err)
		}
	})

	inventory := windowsServiceInventory{manager}
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC)
	var events []monitoring.DiscoveryEvent
	tracker := retirementTracker{client: 7, asset: 17, inventory: inventory,
		publish: func(_ context.Context, batch []monitoring.DiscoveryEvent) error {
			events = append(events, batch...)
			return nil
		}}
	if err := tracker.poll(ctx, now); err != nil || tracker.known[strings.ToLower(name)] != name || len(events) != 0 {
		t.Fatalf("disabled SCM fixture must remain discoverable: %v", err)
	}
	if err := deleteSCMFixture(manager, name); err != nil {
		t.Fatal(err)
	}
	waitForSCMFixtureRemoval(t, inventory, name)

	at := now.Add(time.Minute)
	if err := tracker.poll(ctx, at); err != nil {
		t.Fatal(err)
	}
	want, err := monitoring.NewDiscoveryEvent(7, 17, "services", []string{name}, "tombstone", 100, freshnessSeconds, at)
	if err != nil || len(events) != 1 || events[0] != want {
		t.Fatalf("SCM retirement did not match the fixture identity: %v", err)
	}
	createSCMFixture(t, manager, name, binary)
	if err := tracker.poll(ctx, at.Add(time.Minute)); err != nil || tracker.known[strings.ToLower(name)] != name || len(events) != 1 {
		t.Fatalf("return did not preserve the SCM fixture identity: %v", err)
	}
}

func createSCMFixture(t *testing.T, manager *mgr.Mgr, name, binary string) {
	t.Helper()
	service, err := manager.CreateService(name, binary, mgr.Config{StartType: mgr.StartDisabled}, "/c", "exit", "0")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		_ = deleteSCMFixture(manager, name)
		t.Fatal(err)
	}
}

func deleteSCMFixture(manager *mgr.Mgr, name string) error {
	service, err := manager.OpenService(name)
	if err != nil {
		return err
	}
	return errors.Join(service.Delete(), service.Close())
}

func waitForSCMFixtureRemoval(t *testing.T, inventory windowsServiceInventory, name string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		missing, err := inventory.missing(ctx, name)
		if err == nil && missing {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("SCM did not confirm removal of the test service: %v", err)
		case <-ticker.C:
		}
	}
}
