package usecase

import (
	"context"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type fakeSelfHealReporter struct {
	reports []entities.SelfHealReport
	err     error
}

func (f *fakeSelfHealReporter) ReportSelfHeal(_ context.Context, report entities.SelfHealReport) error {
	f.reports = append(f.reports, report)
	return f.err
}

func TestSelfHealExecutorReloadConfiguration(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	reporter := &fakeSelfHealReporter{}
	called := 0
	exec := NewSelfHealExecutor(log, reporter, SelfHealDeps{
		ConfigSync: func(context.Context) error {
			called++
			return nil
		},
	})

	status, msg, evidence := exec.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "cmd-1",
		Code:      "reload_configuration",
	})

	if status != "success" {
		t.Fatalf("expected success, got %s (%s)", status, msg)
	}
	if called != 1 {
		t.Fatalf("expected config sync called once, got %d", called)
	}
	if len(reporter.reports) != 3 {
		t.Fatalf("expected 3 reports (acked/running/success), got %d", len(reporter.reports))
	}
	if reporter.reports[0].Status != "acked" || reporter.reports[1].Status != "running" || reporter.reports[2].Status != "success" {
		t.Fatalf("unexpected report sequence: %#v", reporter.reports)
	}
	if evidence["duration_ms"] == nil {
		t.Fatalf("expected duration evidence")
	}
}

func TestSelfHealExecutorUnknownCommandFails(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	reporter := &fakeSelfHealReporter{}
	exec := NewSelfHealExecutor(log, reporter, SelfHealDeps{})

	status, _, _ := exec.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "cmd-2",
		Code:      "unknown_action",
	})

	if status != "failed" {
		t.Fatalf("expected failed, got %s", status)
	}
	if len(reporter.reports) != 3 {
		t.Fatalf("expected 3 reports, got %d", len(reporter.reports))
	}
	if reporter.reports[2].Status != "failed" {
		t.Fatalf("expected final failed report, got %s", reporter.reports[2].Status)
	}
}

func TestSelfHealExecutorRestartAgentlessWithoutWorkerFails(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	reporter := &fakeSelfHealReporter{}
	exec := NewSelfHealExecutor(log, reporter, SelfHealDeps{
		HasAgentlessWorker: func() bool { return false },
		RuntimeSnapshot: func() map[string]any {
			return map[string]any{
				"agent_mode_runtime":      "hub",
				"agentless_enabled_env":   true,
				"agentless_enabled_prefs": true,
				"prefs_version":           "cfg-v10",
			}
		},
	})

	status, _, evidence := exec.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "cmd-3",
		Code:      "restart_agentless_worker",
	})
	if status != "failed" {
		t.Fatalf("expected failed, got %s", status)
	}
	if evidence["worker_available"] != false {
		t.Fatalf("expected worker_available=false evidence, got %#v", evidence)
	}
	if evidence["agent_mode_runtime"] != "hub" {
		t.Fatalf("expected runtime mode in evidence, got %#v", evidence)
	}
	if evidence["prefs_version"] != "cfg-v10" {
		t.Fatalf("expected prefs version in evidence, got %#v", evidence)
	}
}

func TestSelfHealExecutorInvalidPayloadWithCommandIDReportsFailed(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	reporter := &fakeSelfHealReporter{}
	exec := NewSelfHealExecutor(log, reporter, SelfHealDeps{})

	status, _, _ := exec.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "cmd-4",
		Code:      "   ",
	})

	if status != "failed" {
		t.Fatalf("expected failed, got %s", status)
	}
	if len(reporter.reports) != 1 {
		t.Fatalf("expected one report, got %d", len(reporter.reports))
	}
	if reporter.reports[0].Status != "failed" {
		t.Fatalf("expected failed report, got %s", reporter.reports[0].Status)
	}
}

func TestSelfHealExecutorInspectRuntimeConfig(t *testing.T) {
	log := logger.New("info")
	t.Cleanup(func() { log.Sync() })
	reporter := &fakeSelfHealReporter{}
	exec := NewSelfHealExecutor(log, reporter, SelfHealDeps{
		RuntimeSnapshot: func() map[string]any {
			return map[string]any{
				"agent_mode_runtime":          "hub",
				"agentless_enabled_env":       false,
				"agentless_enabled_prefs":     true,
				"agentless_effective_enabled": true,
				"worker_available":            true,
			}
		},
	})

	status, msg, evidence := exec.Execute(context.Background(), entities.SelfHealCommand{
		CommandID: "cmd-5",
		Code:      "inspect_runtime_config",
	})

	if status != "success" {
		t.Fatalf("expected success, got %s (%s)", status, msg)
	}
	if evidence["snapshot_source"] != "selfheal.inspect_runtime_config" {
		t.Fatalf("expected snapshot source in evidence, got %#v", evidence)
	}
	if evidence["agent_mode_runtime"] != "hub" {
		t.Fatalf("expected runtime mode in evidence, got %#v", evidence)
	}
	if len(reporter.reports) != 3 {
		t.Fatalf("expected 3 reports (acked/running/success), got %d", len(reporter.reports))
	}
	if reporter.reports[2].Status != "success" {
		t.Fatalf("expected final success report, got %s", reporter.reports[2].Status)
	}
}
