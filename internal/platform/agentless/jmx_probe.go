package agentless

import (
	"context"
	"errors"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func runJMXProbe(ctx context.Context, job entities.AgentlessJob) Result {
	selection, err := monitoring.JMXSelectors(job.Config)
	if err != nil || job.ProbePolicy == nil {
		return Result{Status: "fail", Code: "invalid_jmx_config"}
	}
	settings, ok := config.AgentlessJMXSettings()
	if !ok {
		return Result{Status: "fail", Code: "jmx_worker_unavailable"}
	}
	profile, ok := loadJMXProfile(settings.Profiles, job, selection)
	if !ok {
		return Result{Status: "fail", Code: "jmx_credential_unavailable"}
	}
	receipt, err := invokeJMXWorker(ctx, settings, profile, job.ProbePolicy.BudgetMS)
	if err != nil {
		code := "jmx_worker_unavailable"
		if errors.Is(err, context.DeadlineExceeded) {
			code = "jmx_timeout"
		}
		return Result{Status: "fail", Code: code, failure: err}
	}
	if receipt.Status != "ok" {
		return Result{Status: "fail", Code: "jmx_" + receipt.Code}
	}
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		if value, ok := receipt.Metrics[metric.Preset]; ok {
			values[selection.Reference(metric)] = float64(value)
		}
	}
	return Result{Status: "ok", Payload: map[string]any{"jmx_values_v1": values}}
}
