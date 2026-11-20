package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// One manual command may wait for the current batch. Routine polling yields
// while it waits; collection stays serial and the command deadline includes waiting.
type agentlessCommandGate struct {
	mu       sync.Mutex
	reserved bool
}

func (g *agentlessCommandGate) reserve() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.reserved {
		return false
	}
	g.reserved = true
	return true
}

func (g *agentlessCommandGate) done() {
	g.mu.Lock()
	g.reserved = false
	g.mu.Unlock()
}

func (g *agentlessCommandGate) tryRoutine(busy *atomic.Bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return !g.reserved && busy.CompareAndSwap(false, true)
}

func (g *agentlessCommandGate) acquire(ctx context.Context, busy *atomic.Bool) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if busy.CompareAndSwap(false, true) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func agentlessCommandTimeout(milliseconds int) time.Duration {
	if milliseconds <= 0 {
		return 5 * time.Minute
	}
	if milliseconds > 3600000 {
		return time.Hour
	}
	return time.Duration(milliseconds) * time.Millisecond
}
