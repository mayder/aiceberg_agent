package usecase

import (
	"context"
	"fmt"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func (uc *SelfHealExecutor) inspectRuntimeConfig(ctx context.Context, cmd entities.SelfHealCommand) (string, map[string]any, error) {
	if raw, exists := cmd.Payload["update_readiness"]; exists {
		requested, valid := raw.(bool)
		if !valid || !requested {
			return "invalid update readiness request", nil, fmt.Errorf("update_readiness must be true")
		}
		if uc.deps.UpdateReadiness == nil {
			return "update readiness unavailable", nil, fmt.Errorf("update readiness not supported")
		}
		return "update readiness inspected", map[string]any{"update_readiness": uc.deps.UpdateReadiness(ctx)}, nil
	}
	return "runtime configuration snapshot collected", uc.withRuntimeEvidence(map[string]any{
		"snapshot_source": "selfheal.inspect_runtime_config",
	}), nil
}
