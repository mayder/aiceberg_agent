package app

import (
	"context"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestOutboxCatchupRequiresHubProgressAndUndispatchedWork(t *testing.T) {
	cfg := config.Config{HubOutboxCatchupEnabled: true, OutboxFlushInterval: 15 * time.Second}
	for _, tc := range []struct {
		name, mode      string
		acked, deferred int
		err             error
		want            time.Duration
	}{
		{"progress with deferred groups", "hub", 473, 305, nil, time.Second},
		{"empty queue", "hub", 0, 0, nil, 0},
		{"all requests failed", "hub", 0, 800, nil, 0},
		{"auth or transport retention only", "hub", 473, 0, nil, 0},
		{"parent cancelled", "hub", 473, 305, context.Canceled, 0},
		{"parent deadline", "hub", 473, 305, context.DeadlineExceeded, 0},
		{"relay stays relay", "relay", 473, 305, nil, 0},
		{"direct unchanged", "direct", 473, 305, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := usecase.FlushOutboxSnapshot{LastDeferred: tc.deferred}
			if got := outboxCatchupDelay(tc.mode, cfg, tc.acked, snapshot, tc.err); got != tc.want {
				t.Fatalf("delay=%v want=%v", got, tc.want)
			}
		})
	}
	fullBatch := usecase.FlushOutboxSnapshot{BatchLimit: 800, LastReadBatch: 800}
	if outboxCatchupDelay("hub", cfg, 700, fullBatch, nil) != time.Second {
		t.Fatal("full successful batch still waits while backlog may remain")
	}
	if outboxCatchupDelay("hub", cfg, 0, fullBatch, nil) != 0 {
		t.Fatal("full failed batch caused an aggressive retry")
	}
	deferred := usecase.FlushOutboxSnapshot{LastDeferred: 305}
	cfg.HubOutboxCatchupEnabled = false
	if outboxCatchupDelay("hub", cfg, 473, deferred, nil) != 0 {
		t.Fatal("disabled catch-up still changed timer")
	}
	cfg.HubOutboxCatchupEnabled = true
	for _, interval := range []time.Duration{0, -time.Second, 500 * time.Millisecond, time.Second} {
		cfg.OutboxFlushInterval = interval
		if outboxCatchupDelay("hub", cfg, 473, deferred, nil) != 0 {
			t.Fatal("normal interval was lengthened or invalid interval accepted")
		}
	}
}

func TestOutboxCatchupDoesNotAcceleratePruning(t *testing.T) {
	var cadence outboxFlushCadence
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		elapsed time.Duration
		want    bool
	}{{0, true}, {time.Second, false}, {9 * time.Second, false}, {14 * time.Second, false},
		{15 * time.Second, true}, {16 * time.Second, false}, {30 * time.Second, true}} {
		if got := cadence.pruneDue(start.Add(tc.elapsed), 15*time.Second); got != tc.want {
			t.Fatalf("elapsed=%s prune=%v want=%v", tc.elapsed, got, tc.want)
		}
	}
}
