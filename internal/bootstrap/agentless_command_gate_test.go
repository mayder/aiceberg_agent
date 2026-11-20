package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgentlessCommandWaitsWithoutOverlappingRoutine(t *testing.T) {
	var gate agentlessCommandGate
	var busy atomic.Bool
	if !gate.tryRoutine(&busy) || !gate.reserve() {
		t.Fatal("initial reservation failed")
	}
	if gate.reserve() {
		t.Fatal("second command must be bounded out")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- gate.acquire(ctx, &busy) }()
	select {
	case <-done:
		t.Fatal("command overlapped the routine")
	default:
	}
	var routines sync.WaitGroup
	for i := 0; i < 64; i++ {
		routines.Add(1)
		go func() {
			defer routines.Done()
			if gate.tryRoutine(&busy) {
				t.Error("routine stole reserved command slot")
			}
		}()
	}
	routines.Wait()
	busy.Store(false)
	if gate.tryRoutine(&busy) {
		t.Fatal("routine overtook waiting command")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !busy.Load() || gate.reserve() {
		t.Fatal("running command reservation lost")
	}
	busy.Store(false)
	gate.done()
	if !gate.tryRoutine(&busy) {
		t.Fatal("routine did not resume")
	}
}

func TestAgentlessCommandCancellationLeavesRunningRoutineAlone(t *testing.T) {
	var gate agentlessCommandGate
	var busy atomic.Bool
	busy.Store(true)
	if !gate.reserve() {
		t.Fatal("reservation failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.acquire(ctx, &busy); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	gate.done()
	if !busy.Load() {
		t.Fatal("cancelled waiter cleared another collection lock")
	}
	busy.Store(false)
	if !gate.tryRoutine(&busy) {
		t.Fatal("cancelled waiter blocked routine")
	}
}

func TestAgentlessCommandDeadlineIncludesWaitAndReleasesReservation(t *testing.T) {
	var gate agentlessCommandGate
	var busy atomic.Bool
	busy.Store(true)
	if !gate.reserve() {
		t.Fatal("reservation failed")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := gate.acquire(ctx, &busy); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	gate.done()
	if !gate.reserve() || !busy.Load() {
		t.Fatal("deadline damaged gate state")
	}
	gate.done()
	for _, test := range []struct {
		input int
		want  time.Duration
	}{
		{0, 5 * time.Minute}, {-1, 5 * time.Minute}, {120000, 2 * time.Minute}, {3600001, time.Hour},
	} {
		if got := agentlessCommandTimeout(test.input); got != test.want {
			t.Fatalf("timeout %d: %v", test.input, got)
		}
	}
}
