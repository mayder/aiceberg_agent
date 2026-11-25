package diskretirement

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"golang.org/x/sys/unix"
)

func TestDiskWatcherReportsKernelRemovalOfOwnedLoopDevice(t *testing.T) {
	if os.Getenv("AICEBERG_TEST_DISPOSABLE_LOOP_DEVICE") != "1" {
		t.Skip("requires a disposable Linux block-device fixture")
	}
	name, remove := createDisposableLoop(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const client, asset = 3, 17
	want, err := monitoring.NewDiscoveryEvent(client, asset, "disks", []string{name}, "tombstone", 100, removalFreshnessSec, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	detected, done := startLoopWatcher(ctx, want)
	waitForBlockSubscription(t, ctx, done)
	removedAt := time.Now().UnixMilli()
	remove()
	select {
	case attempts := <-detected:
		event := attempts[1]
		if attempts[0] != event {
			t.Fatalf("retry changed the original tombstone: %+v", attempts)
		}
		if event.ObservedMS < removedAt || event.ObservedMS > time.Now().UnixMilli() || !diskAbsent(blockInventory, name) {
			t.Fatalf("event is outside the confirmed removal: %+v", event)
		}
		event.ObservedMS = want.ObservedMS
		if event != want {
			t.Fatalf("tombstone differs from the canonical disk identity: %+v", event)
		}
	case err := <-done:
		t.Fatalf("watcher stopped before the removal: %v", err)
	case <-ctx.Done():
		t.Fatal("kernel removal was not delivered")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	t.Logf("kernel removed the owned virtual block device %s and the collector retried its original tombstone", name)
}

func startLoopWatcher(ctx context.Context, expected monitoring.DiscoveryEvent) (<-chan [2]monitoring.DiscoveryEvent, <-chan error) {
	detected := make(chan [2]monitoring.DiscoveryEvent, 1)
	done := make(chan error, 1)
	go func() {
		var first monitoring.DiscoveryEvent
		done <- Run(ctx, expected.ClientID, expected.AssetID, func(ctx context.Context, batch []monitoring.DiscoveryEvent) error {
			for _, event := range batch {
				if event.Fingerprint == expected.Fingerprint {
					if first.ObservedMS == 0 {
						first = event
						return errors.New("fixture outbox unavailable")
					}
					select {
					case detected <- [2]monitoring.DiscoveryEvent{first, event}:
					case <-ctx.Done():
					}
				}
			}
			return nil
		})
	}()
	return detected, done
}

func createDisposableLoop(t *testing.T) (string, func()) {
	t.Helper()
	control := filepath.Join(t.TempDir(), "loop-control")
	if err := unix.Mknod(control, unix.S_IFCHR|0600, int(unix.Mkdev(10, 237))); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(control, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	for index := 240; index < 256; index++ {
		name := fmt.Sprintf("loop%d", index)
		if !diskAbsent(blockInventory, name) {
			continue
		}
		if err := unix.IoctlSetInt(fd, unix.LOOP_CTL_ADD, index); errors.Is(err, unix.EEXIST) || errors.Is(err, unix.EBUSY) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		owned := true
		remove := func() {
			if owned {
				if err := unix.IoctlSetInt(fd, unix.LOOP_CTL_REMOVE, index); err != nil {
					t.Fatalf("remove owned fixture %s: %v", name, err)
				}
				owned = false
			}
		}
		t.Cleanup(remove)
		if diskAbsent(blockInventory, name) {
			t.Fatalf("owned device %s was not registered in sysfs", name)
		}
		return name, remove
	}
	t.Fatal("no unused slot for a disposable loop device")
	return "", nil
}

func waitForBlockSubscription(t *testing.T, ctx context.Context, done <-chan error) {
	t.Helper()
	pid := strconv.Itoa(os.Getpid())
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		// Netlink protocol 15 and group 1 are the kernel uevent subscription.
		raw, err := os.ReadFile("/proc/net/netlink")
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[1] == "15" && fields[2] == pid && fields[3] == "00000001" {
				return
			}
		}
		select {
		case err := <-done:
			t.Fatalf("watcher stopped before subscribing: %v", err)
		case <-ctx.Done():
			t.Fatal("kernel uevent subscription was not established")
		case <-tick.C:
		}
	}
}
