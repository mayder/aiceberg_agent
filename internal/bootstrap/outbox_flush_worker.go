package app

import (
	"context"
	"time"
)

func signalOutbox(wake chan<- struct{}) {
	select {
	case wake <- struct{}{}:
	default:
	}
}

// One owner executes both flushes. Arrival signals coalesce in a one-slot
// channel; the periodic timer also recovers persisted work after restart.
func runHubOutboxWorker(ctx context.Context, wake <-chan struct{}, interval time.Duration, execute func(context.Context) outboxCycleResult) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	var notBefore time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-wake:
			if !ok {
				wake = nil
				continue
			}
		case <-timer.C:
		}
		if !waitForOutboxTurn(ctx, notBefore) {
			return
		}
		select {
		case <-wake:
		default:
		}
		timer.Reset(interval)
		result := execute(ctx)
		notBefore = time.Time{}
		if result.acked == 0 && result.failed {
			notBefore = time.Now().Add(time.Second)
		}
		if result.nextDelay > 0 {
			timer.Reset(result.nextDelay)
		}
	}
}

func waitForOutboxTurn(ctx context.Context, notBefore time.Time) bool {
	if delay := time.Until(notBefore); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
		}
	}
	return ctx.Err() == nil
}
