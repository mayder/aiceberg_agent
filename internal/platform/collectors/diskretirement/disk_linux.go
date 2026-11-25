package diskretirement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/collectors/kernelnetlink"
	"golang.org/x/sys/unix"
)

const (
	blockInventory      = "/sys/class/block"
	kernelUeventGroup   = 1
	removalFreshnessSec = 300
	maxRemovalsPerMin   = 16
)

func Run(ctx context.Context, client, asset int64, publish func(context.Context, []monitoring.DiscoveryEvent) error) error {
	if client < 1 || asset < 1 || publish == nil {
		return errors.New("invalid disk discovery scope")
	}
	if !defaultInventoryPath("HOST_PROC", "/proc") || !defaultInventoryPath("HOST_SYS", "/sys") {
		return errors.New("disk retirement requires the local kernel inventory")
	}
	if !blockInventoryAvailable(blockInventory) {
		return errors.New("block inventory unavailable")
	}
	var started time.Time
	remaining := 0
	return kernelnetlink.Receive(ctx, unix.NETLINK_KOBJECT_UEVENT, kernelUeventGroup, func(raw []byte) {
		name := removedDiskName(raw)
		if name == "" {
			return
		}
		now := time.Now()
		if started.IsZero() || now.Sub(started) >= time.Minute {
			started, remaining = now, maxRemovalsPerMin
		}
		if remaining > 0 {
			remaining--
			event, err := monitoring.NewDiscoveryEvent(client, asset, "disks", []string{name}, "tombstone", 100, removalFreshnessSec, now)
			if err == nil {
				publishRemovedDisk(ctx, name, event, publish)
			}
		}
	})
}

func defaultInventoryPath(environment, standard string) bool {
	path := os.Getenv(environment)
	return path == "" || filepath.Clean(path) == standard
}

func blockInventoryAvailable(root string) bool {
	info, err := os.Stat(root)
	return err == nil && info.IsDir()
}

func diskAbsent(root, name string) bool {
	if !diskName.MatchString(name) || !blockInventoryAvailable(root) {
		return false
	}
	// Lstat also recognizes block symlinks whose target is in transition.
	_, err := os.Lstat(filepath.Join(root, name))
	return errors.Is(err, os.ErrNotExist) && blockInventoryAvailable(root)
}

func publishRemovedDisk(ctx context.Context, name string, event monitoring.DiscoveryEvent,
	publish func(context.Context, []monitoring.DiscoveryEvent) error,
) {
	// The notification may precede removal of the sysfs entry.
	settle := time.NewTimer(300 * time.Millisecond)
	defer settle.Stop()
	select {
	case <-ctx.Done():
		return
	case <-settle.C:
	}
	backoff := time.NewTicker(5 * time.Second)
	defer backoff.Stop()
	for ctx.Err() == nil && time.Now().UnixMilli()-event.ObservedMS < removalFreshnessSec*1000 {
		if !diskAbsent(blockInventory, name) || publish(ctx, []monitoring.DiscoveryEvent{event}) == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-backoff.C:
		}
	}
}
