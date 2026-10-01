package schedule

import (
	"testing"
	"time"
)

func TestInitialDelayIsDeterministicAndBounded(t *testing.T) {
	interval := 30 * time.Second
	got := InitialDelay("install-01", "config-sync", interval)
	if got != InitialDelay("install-01", "config-sync", interval) {
		t.Fatal("same identity and routine must produce the same delay")
	}
	if got < 0 || got >= interval {
		t.Fatalf("delay must be in [0, interval), got %s", got)
	}
}

func TestInitialDelayDistributesIdentitiesAndRoutines(t *testing.T) {
	interval := 30 * time.Second
	values := map[time.Duration]struct{}{}
	for _, tc := range []struct{ identity, routine string }{
		{"install-01", "ping"}, {"install-02", "ping"}, {"install-01", "config-sync"}, {"install-01", "self-heal"},
	} {
		values[InitialDelay(tc.identity, tc.routine, interval)] = struct{}{}
	}
	if len(values) < 3 {
		t.Fatalf("expected distributed phases, got %d distinct values", len(values))
	}
}

func TestPeriodicTimerKeepsConfiguredInterval(t *testing.T) {
	interval := 15 * time.Second
	timer := NewPeriodicTimer("install-01", "flush", interval)
	defer timer.Stop()
	if timer.interval != interval {
		t.Fatalf("configured interval changed: got %s want %s", timer.interval, interval)
	}
}
