package interfaceretirement

import (
	"context"
	"testing"
)

func TestWindowsWatcherRegistersAndCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 2 {
		if err := watchRemovals(ctx, func([]removal) {}); err != nil {
			t.Fatalf("register/cancel interface notification: %v", err)
		}
	}
}
