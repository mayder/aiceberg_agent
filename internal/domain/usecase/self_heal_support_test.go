package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestSupportFlareMissingAndPartialEvidence(t *testing.T) {
	for _, missing := range []bool{true, false} {
		reporter := &fakeSelfHealReporter{}
		deps := SelfHealDeps{}
		if !missing {
			deps.SupportFlare = func(context.Context) (map[string]any, error) {
				return map[string]any{"status": "unavailable", "source": "agent_service_journal"}, errors.New("unavailable")
			}
		}
		executor := NewSelfHealExecutor(testLogger{}, reporter, deps)
		status, _, evidence := executor.Execute(context.Background(), entities.SelfHealCommand{CommandID: "support", Code: "collect_support_flare"})
		if status != "failed" || reporter.reports[len(reporter.reports)-1].Status != "failed" {
			t.Fatal(status)
		}
		if !missing && evidence["flare"] == nil {
			t.Fatal("partial evidence discarded")
		}
	}
}
