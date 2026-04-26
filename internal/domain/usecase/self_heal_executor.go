package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type SelfHealReporter interface {
	ReportSelfHeal(ctx context.Context, report entities.SelfHealReport) error
}

type SelfHealDeps struct {
	ConfigSync          func(context.Context) error
	Ping                func(context.Context) error
	ApplyAgentMode      func(context.Context, entities.SelfHealCommand) (string, map[string]any, error)
	AgentlessSync       func(context.Context) error
	AgentlessCollectNow func(context.Context)
	AgentlessFlush      func(context.Context) error
	CollectMetrics      func(context.Context) error
	CollectHealth       func(context.Context) error
	CollectInventory    func(context.Context) error
	CollectBootstrap    func(context.Context) error
	CollectNetwork      func(context.Context) error
	ClearAgentlessLock  func()
	HasAgentlessWorker  func() bool
	RuntimeSnapshot     func() map[string]any
}

type SelfHealExecutor struct {
	log      logger.Logger
	reporter SelfHealReporter
	deps     SelfHealDeps
}

func NewSelfHealExecutor(log logger.Logger, reporter SelfHealReporter, deps SelfHealDeps) *SelfHealExecutor {
	return &SelfHealExecutor{log: log, reporter: reporter, deps: deps}
}

func (uc *SelfHealExecutor) Execute(ctx context.Context, cmd entities.SelfHealCommand) (string, string, map[string]any) {
	commandID := strings.TrimSpace(cmd.CommandID)
	code := strings.TrimSpace(strings.ToLower(cmd.Code))
	if commandID == "" {
		return "failed", "invalid command payload", map[string]any{"code": "invalid_command_payload"}
	}
	if code == "" {
		evidence := map[string]any{"code": "invalid_command_payload"}
		_ = uc.report(ctx, cmd, "failed", "invalid command payload", evidence)
		return "failed", "invalid command payload", evidence
	}

	_ = uc.report(ctx, cmd, "acked", "command acknowledged", nil)
	_ = uc.report(ctx, cmd, "running", "command running", nil)

	started := time.Now()
	msg, evidence, err := uc.executeCode(ctx, cmd, code)
	durationMs := time.Since(started).Milliseconds()
	if evidence == nil {
		evidence = map[string]any{}
	}
	evidence["duration_ms"] = durationMs
	evidence["command_code"] = code

	if err != nil {
		if msg == "" {
			msg = err.Error()
		}
		uc.log.Error(logger.KV("selfheal command failed",
			"command_id", commandID,
			"command_code", code,
			"err", err,
		))
		_ = uc.report(ctx, cmd, "failed", msg, evidence)
		return "failed", msg, evidence
	}

	if strings.TrimSpace(msg) == "" {
		msg = "self-healing command completed"
	}
	uc.log.Info(logger.KV("selfheal command success",
		"command_id", commandID,
		"command_code", code,
		"duration_ms", durationMs,
	))
	_ = uc.report(ctx, cmd, "success", msg, evidence)
	return "success", msg, evidence
}

func (uc *SelfHealExecutor) report(ctx context.Context, cmd entities.SelfHealCommand, status, message string, evidence map[string]any) error {
	if uc.reporter == nil {
		return nil
	}
	report := entities.NewSelfHealReport(strings.TrimSpace(cmd.CommandID), status)
	report.Message = strings.TrimSpace(message)
	report.CorrelationID = strings.TrimSpace(cmd.CorrelationID)
	if len(evidence) > 0 {
		report.Evidence = evidence
	}
	if err := uc.reporter.ReportSelfHeal(ctx, report); err != nil {
		uc.log.Error(logger.KV("selfheal report failed",
			"command_id", cmd.CommandID,
			"status", status,
			"err", err,
		))
		return err
	}
	return nil
}

func (uc *SelfHealExecutor) executeCode(ctx context.Context, cmd entities.SelfHealCommand, code string) (string, map[string]any, error) {
	switch code {
	case "apply_agent_mode":
		if uc.deps.ApplyAgentMode == nil {
			return "agent mode change dependency unavailable", nil, fmt.Errorf("agent mode change dependency unavailable")
		}
		return uc.deps.ApplyAgentMode(ctx, cmd)
	case "restart_agentless_worker":
		if uc.deps.HasAgentlessWorker != nil && !uc.deps.HasAgentlessWorker() {
			evidence := uc.withRuntimeEvidence(map[string]any{"worker_available": false})
			return "agentless worker unavailable in current mode", evidence, fmt.Errorf("agentless unavailable")
		}
		if uc.deps.AgentlessSync != nil {
			if err := uc.deps.AgentlessSync(ctx); err != nil {
				return "failed to sync agentless targets", nil, err
			}
		}
		if uc.deps.AgentlessCollectNow != nil {
			uc.deps.AgentlessCollectNow(ctx)
		}
		if uc.deps.AgentlessFlush != nil {
			if err := uc.deps.AgentlessFlush(ctx); err != nil {
				return "failed to flush agentless outbox", nil, err
			}
		}
		return "agentless worker cycle restarted", map[string]any{"worker": "agentless"}, nil
	case "reload_configuration":
		if uc.deps.ConfigSync == nil {
			return "config sync dependency unavailable", nil, fmt.Errorf("config sync dependency unavailable")
		}
		if err := uc.deps.ConfigSync(ctx); err != nil {
			return "failed to reload remote configuration", nil, err
		}
		return "configuration reloaded from backend", map[string]any{"route": "/v1/agent/config"}, nil
	case "clear_local_lock":
		if uc.deps.ClearAgentlessLock != nil {
			uc.deps.ClearAgentlessLock()
		}
		return "local lock state cleared", map[string]any{"lock": "agentless_busy"}, nil
	case "requeue_pending_collect":
		run := func(fn func(context.Context) error) error {
			if fn == nil {
				return nil
			}
			return fn(ctx)
		}
		if err := run(uc.deps.CollectMetrics); err != nil {
			return "failed to requeue metrics collect", nil, err
		}
		if err := run(uc.deps.CollectHealth); err != nil {
			return "failed to requeue health collect", nil, err
		}
		if err := run(uc.deps.CollectInventory); err != nil {
			return "failed to requeue inventory collect", nil, err
		}
		if err := run(uc.deps.CollectBootstrap); err != nil {
			return "failed to requeue bootstrap collect", nil, err
		}
		if err := run(uc.deps.CollectNetwork); err != nil {
			return "failed to requeue network capture", nil, err
		}
		return "collect queues re-triggered", map[string]any{"collect": []string{"metrics", "health", "inventory", "bootstrap", "network_capture"}}, nil
	case "validate_api_connectivity":
		if uc.deps.Ping != nil {
			if err := uc.deps.Ping(ctx); err != nil {
				return "ping connectivity check failed", nil, err
			}
		}
		if uc.deps.ConfigSync != nil {
			if err := uc.deps.ConfigSync(ctx); err != nil {
				return "config connectivity check failed", nil, err
			}
		}
		return "api connectivity validated", map[string]any{"checks": []string{"ping", "config"}}, nil
	case "resync_clock":
		if uc.deps.CollectHealth != nil {
			if err := uc.deps.CollectHealth(ctx); err != nil {
				return "time sync probe failed", nil, err
			}
		}
		return "time sync probe triggered", map[string]any{"source": "health_collect"}, nil
	case "inspect_runtime_config":
		return "runtime configuration snapshot collected", uc.withRuntimeEvidence(map[string]any{
			"snapshot_source": "selfheal.inspect_runtime_config",
		}), nil
	default:
		return "unsupported self-healing command", map[string]any{"command_code": code}, fmt.Errorf("unsupported command: %s", code)
	}
}

func (uc *SelfHealExecutor) withRuntimeEvidence(base map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	if uc.deps.RuntimeSnapshot == nil {
		return out
	}
	snapshot := uc.deps.RuntimeSnapshot()
	for k, v := range snapshot {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}
