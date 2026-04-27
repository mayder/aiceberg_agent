package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/channel"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

const (
	defaultChannelCommandTimeout = 5 * time.Minute
	defaultChannelRetryAfter     = 2 * time.Second
	maxChannelCommandAttempts    = 5
)

type ChannelEventSender interface {
	SendEvent(context.Context, channel.Envelope) error
}

type ChannelCommandExecutionResult struct {
	Status   string
	Message  string
	Evidence map[string]any
	Executed bool
}

func ChannelEnvelopeToSelfHealCommand(env channel.Envelope) entities.SelfHealCommand {
	payload := map[string]any{}
	if env.Payload != nil {
		if nested, ok := env.Payload["payload"].(map[string]any); ok {
			payload = nested
		}
	}
	return entities.SelfHealCommand{
		CommandID:     strings.TrimSpace(env.CommandID),
		Code:          stringFromMap(env.Payload, "code"),
		Mode:          stringFromMap(env.Payload, "mode"),
		Payload:       payload,
		CorrelationID: strings.TrimSpace(env.CorrelationID),
		TriggerRule:   stringFromMap(env.Payload, "trigger_rule"),
		RequestedAt:   stringFromMap(env.Payload, "requested_at"),
	}
}

func ExecuteSelfHealOnce(
	ctx context.Context,
	dedupe *CommandIdempotency,
	executor *SelfHealExecutor,
	command entities.SelfHealCommand,
) (string, string, map[string]any, bool) {
	if dedupe != nil && !dedupe.First(command.CommandID) {
		return "duplicate", "duplicate command ignored", map[string]any{"command_id": command.CommandID}, false
	}
	status, message, evidence := executor.Execute(ctx, command)
	return status, message, evidence, true
}

func ExecuteChannelSelfHealCommand(
	ctx context.Context,
	dedupe *CommandIdempotency,
	executor *SelfHealExecutor,
	sender ChannelEventSender,
	env channel.Envelope,
) ChannelCommandExecutionResult {
	command := ChannelEnvelopeToSelfHealCommand(env)
	if dedupe != nil && !dedupe.First(command.CommandID) {
		sendChannelExecutionEvent(ctx, sender, env, channel.TypeAck, map[string]any{
			"status": "duplicate",
			"stage":  "dedupe",
		}, 0, 0)
		return ChannelCommandExecutionResult{
			Status:   "duplicate",
			Message:  "duplicate command ignored",
			Evidence: map[string]any{"command_id": command.CommandID},
			Executed: false,
		}
	}

	timeout := channelCommandTimeout(env)
	retryable := channelCommandRetryable(env)
	maxAttempts := channelCommandMaxAttempts(env, retryable)
	retryAfter := channelCommandRetryAfter(env)

	sendChannelExecutionEvent(ctx, sender, env, channel.TypeAck, map[string]any{
		"status":       channel.StatusAccepted,
		"stage":        "ack",
		"retryable":    retryable,
		"max_attempts": maxAttempts,
		"timeout_ms":   timeout.Milliseconds(),
	}, 1, 0)

	var last ChannelCommandExecutionResult
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		sendChannelExecutionEvent(ctx, sender, env, channel.TypeProgress, map[string]any{
			"status":       channel.StatusRunning,
			"stage":        "running",
			"attempt":      attempt,
			"max_attempts": maxAttempts,
			"timeout_ms":   timeout.Milliseconds(),
		}, attempt, 0)

		status, message, evidence := executor.Execute(attemptCtx, command)
		timedOut := attemptCtx.Err() == context.DeadlineExceeded
		cancel()
		if evidence == nil {
			evidence = map[string]any{}
		}
		evidence["attempt"] = attempt
		evidence["max_attempts"] = maxAttempts
		evidence["retryable"] = retryable

		if timedOut {
			message = "command timed out"
			evidence["failure_class"] = "timeout"
			evidence["timeout_ms"] = timeout.Milliseconds()
			sendChannelExecutionEvent(ctx, sender, env, channel.TypeTimeout, map[string]any{
				"status":        channel.StatusTimeout,
				"stage":         "timeout",
				"failure_class": "timeout",
				"attempt":       attempt,
				"max_attempts":  maxAttempts,
				"timeout_ms":    timeout.Milliseconds(),
			}, attempt, 0)
			status = channel.StatusTimeout
		}

		last = ChannelCommandExecutionResult{
			Status:   status,
			Message:  message,
			Evidence: evidence,
			Executed: true,
		}
		if status == channel.StatusSuccess || status == "success" {
			sendChannelExecutionEvent(ctx, sender, env, channel.TypeResult, map[string]any{
				"status":       channel.StatusSuccess,
				"stage":        "completed",
				"attempt":      attempt,
				"max_attempts": maxAttempts,
				"message":      message,
				"evidence":     evidence,
			}, attempt, 0)
			last.Status = "success"
			return last
		}

		if retryable && attempt < maxAttempts && ctx.Err() == nil {
			sendChannelExecutionEvent(ctx, sender, env, channel.TypeRetry, map[string]any{
				"status":         channel.StatusRetrying,
				"stage":          "retrying",
				"attempt":        attempt,
				"next_attempt":   attempt + 1,
				"max_attempts":   maxAttempts,
				"retry_after_ms": retryAfter.Milliseconds(),
				"message":        message,
			}, attempt, int(retryAfter.Milliseconds()))
			if !sleepChannelRetry(ctx, retryAfter) {
				break
			}
			continue
		}

		sendChannelExecutionEvent(ctx, sender, env, channel.TypeError, map[string]any{
			"status":       channel.StatusFailed,
			"stage":        "failed",
			"attempt":      attempt,
			"max_attempts": maxAttempts,
			"message":      message,
			"evidence":     evidence,
		}, attempt, 0)
		last.Status = "failed"
		return last
	}

	if last.Status == "" {
		last = ChannelCommandExecutionResult{
			Status:   "failed",
			Message:  "command cancelled before completion",
			Evidence: map[string]any{"failure_class": "cancelled"},
			Executed: true,
		}
	}
	return last
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if scalar, ok := value.(string); ok {
		return strings.TrimSpace(scalar)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func sendChannelExecutionEvent(ctx context.Context, sender ChannelEventSender, source channel.Envelope, eventType string, body map[string]any, attempt int, retryAfterMs int) {
	if sender == nil {
		return
	}
	env := channel.Envelope{
		Type:          eventType,
		CommandID:     strings.TrimSpace(source.CommandID),
		CorrelationID: strings.TrimSpace(source.CorrelationID),
		Attempt:       attempt,
		RetryAfterMs:  retryAfterMs,
	}
	switch eventType {
	case channel.TypeAck, channel.TypeTimeout, channel.TypeRetry:
		env.Payload = body
	case channel.TypeProgress:
		env.Progress = body
	case channel.TypeResult:
		env.Result = body
	case channel.TypeError:
		env.Error = body
	default:
		env.Payload = body
	}
	_ = sender.SendEvent(ctx, env)
}

func channelCommandTimeout(env channel.Envelope) time.Duration {
	timeoutMs := env.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = intFromMap(env.Payload, "timeout_ms")
	}
	if timeoutMs <= 0 {
		return defaultChannelCommandTimeout
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

func channelCommandRetryAfter(env channel.Envelope) time.Duration {
	retryAfterMs := env.RetryAfterMs
	if retryAfterMs <= 0 {
		retryAfterMs = intFromMap(env.Payload, "retry_after_ms")
	}
	if retryAfterMs <= 0 {
		return defaultChannelRetryAfter
	}
	return time.Duration(retryAfterMs) * time.Millisecond
}

func channelCommandRetryable(env channel.Envelope) bool {
	if boolFromMap(env.Payload, "retryable") {
		return true
	}
	if retry, ok := env.Payload["retry"].(map[string]any); ok {
		return boolFromMap(retry, "enabled") || boolFromMap(retry, "retryable")
	}
	return false
}

func channelCommandMaxAttempts(env channel.Envelope, retryable bool) int {
	if !retryable {
		return 1
	}
	maxAttempts := intFromMap(env.Payload, "max_attempts")
	if retry, ok := env.Payload["retry"].(map[string]any); ok && maxAttempts <= 0 {
		maxAttempts = intFromMap(retry, "max_attempts")
	}
	if maxAttempts < 2 {
		maxAttempts = 2
	}
	if maxAttempts > maxChannelCommandAttempts {
		maxAttempts = maxChannelCommandAttempts
	}
	return maxAttempts
}

func boolFromMap(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	switch value := values[key].(type) {
	case bool:
		return value
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "sim"
	default:
		return fmt.Sprint(value) == "1"
	}
}

func intFromMap(values map[string]any, key string) int {
	if values == nil {
		return 0
	}
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case jsonNumber:
		i, _ := value.Int64()
		return int(i)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

type jsonNumber interface {
	Int64() (int64, error)
}

func sleepChannelRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
