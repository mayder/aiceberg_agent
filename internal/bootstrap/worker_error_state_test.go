package app

import (
	"errors"
	"testing"
	"time"
)

func TestWorkerErrorStateMachineInitialSuccessDoesNotRecover(t *testing.T) {
	clock := time.Unix(100, 0)
	machine := newWorkerErrorStateMachine(func() time.Time { return clock })
	calls := 0
	sent, err := machine.report("fp", "recovered", func(time.Time) error { calls++; return nil })
	if err != nil || sent || calls != 0 {
		t.Fatalf("initial healthy cycle reported recovery: sent=%v calls=%d err=%v", sent, calls, err)
	}
}

func TestWorkerErrorStateMachineOpenThenRecoversExactlyOnce(t *testing.T) {
	clock := time.Unix(100, 0)
	machine := newWorkerErrorStateMachine(func() time.Time { return clock })
	calls := map[string]int{}
	send := func(state string) func(time.Time) error { return func(time.Time) error { calls[state]++; return nil } }
	if sent, err := machine.report("fp", "open", send("open")); !sent || err != nil {
		t.Fatalf("open not sent: sent=%v err=%v", sent, err)
	}
	if sent, err := machine.report("fp", "recovered", send("recovered")); !sent || err != nil {
		t.Fatalf("recovery not sent: sent=%v err=%v", sent, err)
	}
	clock = clock.Add(2 * time.Hour)
	for range 3 {
		if sent, err := machine.report("fp", "recovered", send("recovered")); sent || err != nil {
			t.Fatalf("repeated success reported recovery: sent=%v err=%v", sent, err)
		}
	}
	if calls["open"] != 1 || calls["recovered"] != 1 {
		t.Fatalf("unexpected sends: %#v", calls)
	}
}

func TestWorkerErrorStateMachineRepeatedFailuresRespectCooldown(t *testing.T) {
	clock := time.Unix(100, 0)
	machine := newWorkerErrorStateMachine(func() time.Time { return clock })
	calls := 0
	send := func(time.Time) error { calls++; return nil }
	_, _ = machine.report("fp", "open", send)
	clock = clock.Add(29 * time.Minute)
	if sent, _ := machine.report("fp", "open", send); sent {
		t.Fatal("open repeated inside cooldown")
	}
	clock = clock.Add(time.Minute)
	if sent, err := machine.report("fp", "open", send); !sent || err != nil {
		t.Fatalf("open not repeated after cooldown: sent=%v err=%v", sent, err)
	}
	if calls != 2 {
		t.Fatalf("expected two open deliveries, got %d", calls)
	}
}

func TestWorkerErrorStateMachineFailedOpenDoesNotCreateRecovery(t *testing.T) {
	clock := time.Unix(100, 0)
	machine := newWorkerErrorStateMachine(func() time.Time { return clock })
	failure := errors.New("offline")
	if sent, err := machine.report("fp", "open", func(time.Time) error { return failure }); !sent || !errors.Is(err, failure) {
		t.Fatalf("failed open attempt mismatch: sent=%v err=%v", sent, err)
	}
	if sent, err := machine.report("fp", "recovered", func(time.Time) error { return nil }); sent || err != nil {
		t.Fatalf("failed open created false recovery: sent=%v err=%v", sent, err)
	}
	if sent, _ := machine.report("fp", "open", func(time.Time) error { return nil }); sent {
		t.Fatal("failed open retried inside cooldown")
	}
}
