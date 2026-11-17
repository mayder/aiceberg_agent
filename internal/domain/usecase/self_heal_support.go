package usecase

import (
	"context"
	"errors"
)

func (uc *SelfHealExecutor) collectSupportFlare(ctx context.Context) (string, map[string]any, error) {
	evidence := uc.withRuntimeEvidence(map[string]any{
		"snapshot_source": "selfheal.collect_support_flare", "flare_kind": "support",
	})
	if uc.deps.SupportFlare == nil {
		evidence["support_status"] = "unsupported"
		return "support collector unavailable", sanitizeEvidence(evidence), errors.New("support collector unavailable")
	}
	flare, err := uc.deps.SupportFlare(ctx)
	evidence["flare"] = flare
	if err != nil {
		return "support flare collection failed", sanitizeEvidence(evidence), err
	}
	if len(flare) == 0 {
		return "support evidence unavailable", sanitizeEvidence(evidence), errors.New("empty support evidence")
	}
	return "support flare collected", sanitizeEvidence(evidence), nil
}
