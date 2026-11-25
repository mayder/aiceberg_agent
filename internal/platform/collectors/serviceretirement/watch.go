//go:build linux || windows

package serviceretirement

import (
	"context"
	"time"
)

func (r *retirementTracker) watch(ctx context.Context) error {
	_ = r.poll(ctx, time.Now())
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Each poll revalidates pending removals before retrying the outbox.
			_ = r.poll(ctx, time.Now())
		}
	}
}
