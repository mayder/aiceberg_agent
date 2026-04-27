package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/channel"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type fakeChannelEventSender struct {
	events []channel.Envelope
}

func (f *fakeChannelEventSender) SendEvent(_ context.Context, env channel.Envelope) error {
	f.events = append(f.events, env)
	return nil
}

func TestChannelEnvelopeToSelfHealCommand(t *testing.T) {
	command := ChannelEnvelopeToSelfHealCommand(channel.Envelope{
		CommandID:     "cmd-1",
		CorrelationID: "corr-1",
		Payload: map[string]any{
			"code":         "inspect_runtime_config",
			"mode":         "automatico",
			"trigger_rule": "channel",
			"requested_at": "2026-04-26T12:00:00Z",
			"payload": map[string]any{
				"scope": "runtime",
			},
		},
	})

	if command.CommandID != "cmd-1" || command.Code != "inspect_runtime_config" || command.CorrelationID != "corr-1" {
		t.Fatalf("unexpected command %#v", command)
	}
	if command.Payload["scope"] != "runtime" {
		t.Fatalf("unexpected payload %#v", command.Payload)
	}
}

func TestExecuteSelfHealOnceSkipsDuplicateCommandID(t *testing.T) {
	dedupe := NewCommandIdempotency(time.Hour, 10)
	executor := NewSelfHealExecutor(testLogger{}, &fakeSelfHealReporter{}, SelfHealDeps{
		RuntimeSnapshot: func() map[string]any {
			return map[string]any{"agent_mode_runtime": "direct"}
		},
	})
	command := entities.SelfHealCommand{
		CommandID: "cmd-dup",
		Code:      "inspect_runtime_config",
	}

	_, _, _, first := ExecuteSelfHealOnce(context.Background(), dedupe, executor, command)
	status, _, _, second := ExecuteSelfHealOnce(context.Background(), dedupe, executor, command)

	if !first {
		t.Fatalf("expected first command to execute")
	}
	if second || status != "duplicate" {
		t.Fatalf("expected duplicate skip, status=%s executed=%v", status, second)
	}
}

func TestExecuteChannelSelfHealCommandAppliesTimeoutAndCooperativeCancel(t *testing.T) {
	dedupe := NewCommandIdempotency(time.Hour, 10)
	reporter := &fakeSelfHealReporter{}
	sender := &fakeChannelEventSender{}
	cancelled := make(chan struct{}, 1)
	executor := NewSelfHealExecutor(testLogger{}, reporter, SelfHealDeps{
		ConfigSync: func(ctx context.Context) error {
			<-ctx.Done()
			cancelled <- struct{}{}
			return ctx.Err()
		},
	})

	result := ExecuteChannelSelfHealCommand(context.Background(), dedupe, executor, sender, channel.Envelope{
		CommandID:     "cmd-timeout",
		CorrelationID: "corr-timeout",
		TimeoutMs:     10,
		Payload: map[string]any{
			"code": "reload_configuration",
		},
	})

	if result.Status != "failed" || result.Evidence["failure_class"] != "timeout" {
		t.Fatalf("expected timeout failure, got %#v", result)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatalf("expected cooperative context cancellation")
	}
	if !eventTypes(sender.events, channel.TypeAck, channel.TypeProgress, channel.TypeTimeout, channel.TypeError) {
		t.Fatalf("unexpected channel events: %#v", sender.events)
	}
}

func TestExecuteChannelSelfHealCommandRetriesOnlyWhenRetryable(t *testing.T) {
	dedupe := NewCommandIdempotency(time.Hour, 10)
	sender := &fakeChannelEventSender{}
	attempts := 0
	executor := NewSelfHealExecutor(testLogger{}, &fakeSelfHealReporter{}, SelfHealDeps{
		Ping: func(context.Context) error {
			attempts++
			if attempts == 1 {
				return errors.New("temporary connectivity failure")
			}
			return nil
		},
	})

	result := ExecuteChannelSelfHealCommand(context.Background(), dedupe, executor, sender, channel.Envelope{
		CommandID:    "cmd-retry",
		RetryAfterMs: 1,
		Payload: map[string]any{
			"code":         "validate_api_connectivity",
			"retryable":    true,
			"max_attempts": 2,
		},
	})

	if result.Status != "success" {
		t.Fatalf("expected success after retry, got %#v", result)
	}
	if attempts != 2 {
		t.Fatalf("expected two attempts, got %d", attempts)
	}
	if !eventTypes(sender.events, channel.TypeAck, channel.TypeProgress, channel.TypeRetry, channel.TypeProgress, channel.TypeResult) {
		t.Fatalf("unexpected channel events: %#v", sender.events)
	}
}

func TestExecuteChannelSelfHealCommandDoesNotRetryWithoutRetryableFlag(t *testing.T) {
	dedupe := NewCommandIdempotency(time.Hour, 10)
	sender := &fakeChannelEventSender{}
	attempts := 0
	executor := NewSelfHealExecutor(testLogger{}, &fakeSelfHealReporter{}, SelfHealDeps{
		Ping: func(context.Context) error {
			attempts++
			return errors.New("connectivity failure")
		},
	})

	result := ExecuteChannelSelfHealCommand(context.Background(), dedupe, executor, sender, channel.Envelope{
		CommandID: "cmd-no-retry",
		Payload: map[string]any{
			"code":         "validate_api_connectivity",
			"max_attempts": 3,
		},
	})

	if result.Status != "failed" {
		t.Fatalf("expected failed without retry, got %#v", result)
	}
	if attempts != 1 {
		t.Fatalf("expected single attempt, got %d", attempts)
	}
	if eventTypeCount(sender.events, channel.TypeRetry) != 0 {
		t.Fatalf("non-retryable command must not emit retry: %#v", sender.events)
	}
}

func TestExecuteChannelSelfHealCommandDedupeBlocksSecondDelivery(t *testing.T) {
	dedupe := NewCommandIdempotency(time.Hour, 10)
	sender := &fakeChannelEventSender{}
	calls := 0
	executor := NewSelfHealExecutor(testLogger{}, &fakeSelfHealReporter{}, SelfHealDeps{
		RuntimeSnapshot: func() map[string]any {
			calls++
			return map[string]any{"agent_mode_runtime": "direct"}
		},
	})
	env := channel.Envelope{
		CommandID: "cmd-dedupe",
		Payload: map[string]any{
			"code": "inspect_runtime_config",
		},
	}

	first := ExecuteChannelSelfHealCommand(context.Background(), dedupe, executor, sender, env)
	second := ExecuteChannelSelfHealCommand(context.Background(), dedupe, executor, sender, env)

	if first.Status != "success" || second.Executed || second.Status != "duplicate" {
		t.Fatalf("unexpected results first=%#v second=%#v", first, second)
	}
	if calls != 1 {
		t.Fatalf("expected command body to execute once, got %d", calls)
	}
}

func eventTypes(events []channel.Envelope, types ...string) bool {
	if len(events) != len(types) {
		return false
	}
	for i := range types {
		if events[i].Type != types[i] {
			return false
		}
	}
	return true
}

func eventTypeCount(events []channel.Envelope, typ string) int {
	total := 0
	for _, event := range events {
		if event.Type == typ {
			total++
		}
	}
	return total
}
