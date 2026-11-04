package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestHubOutboxWorkerCoalescesArrivalsAndNeverOverlapsCycles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	started, release := make(chan struct{}, 4), make(chan struct{}, 2)
	var active, max atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHubOutboxWorker(ctx, wake, time.Hour, func(ctx context.Context) outboxCycleResult {
			n := active.Add(1)
			defer active.Add(-1)
			if n > 1 {
				max.Store(n)
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
			return outboxCycleResult{acked: 1}
		})
	}()
	awaitOutboxSignal(t, started, "startup recovery")
	for i := 0; i < 1000; i++ {
		signalOutbox(wake)
	}
	if len(wake) != 1 || active.Load() != 1 {
		t.Fatal("burst blocked receiver or spawned extra workers")
	}
	release <- struct{}{}
	awaitOutboxSignal(t, started, "arrival while sending")
	if max.Load() != 0 || active.Load() != 1 {
		t.Fatal("flush cycles overlapped")
	}
	cancel()
	awaitOutboxSignal(t, done, "worker shutdown")
}

func TestHubOutboxWorkerFailureGuardIsCancellable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wake := make(chan struct{}, 1)
	called := make(chan struct{}, 3)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHubOutboxWorker(ctx, wake, time.Hour, func(context.Context) outboxCycleResult {
			called <- struct{}{}
			return outboxCycleResult{failed: true}
		})
	}()
	awaitOutboxSignal(t, called, "first attempt")
	signalOutbox(wake)
	select {
	case <-called:
		t.Fatal("arrival storm bypassed local no-progress guard")
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	awaitOutboxSignal(t, done, "cancel during failure guard")
}

func TestHubOutboxWorkerPeriodicFallbackWithoutNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := make(chan struct{}, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHubOutboxWorker(ctx, nil, 5*time.Millisecond, func(context.Context) outboxCycleResult {
			called <- struct{}{}
			return outboxCycleResult{}
		})
	}()
	awaitOutboxSignal(t, called, "startup")
	awaitOutboxSignal(t, called, "periodic fallback")
	cancel()
	awaitOutboxSignal(t, done, "periodic shutdown")
}

func awaitOutboxSignal(t *testing.T, ch <-chan struct{}, stage string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out: %s", stage)
	}
}
