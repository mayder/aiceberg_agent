package diskretirement

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestDiskAbsenceRequiresAnAvailableInventory(t *testing.T) {
	root := t.TempDir()
	if diskAbsent(filepath.Join(root, "unmounted-sysfs"), "sda") {
		t.Fatal("missing inventory mistaken for removal")
	}
	link := filepath.Join(root, "sda")
	if err := os.Symlink(filepath.Join(root, "kernel-object"), link); err != nil {
		t.Fatal(err)
	}
	if diskAbsent(root, "sda") {
		t.Fatal("dangling sysfs link mistaken for removal")
	}
	if !diskAbsent(root, "sdb2") || diskAbsent(root, "../sdb2") {
		t.Fatal("absence must be confirmed using a valid I/O device identity")
	}
}

func TestDiskWatcherRegistersWithoutMutatingKernelState(t *testing.T) {
	if os.Getenv("AICEBERG_TEST_DISPOSABLE_LINK_NAMESPACE") != "1" {
		t.Skip("requires the isolated Linux collector fixture")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, 3, 17, func(context.Context, []monitoring.DiscoveryEvent) error {
		t.Fatal("watcher published during registration")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
