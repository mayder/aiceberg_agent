package usecase

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/data/local/configstate"
)

func (uc *ConfigSync) restoreConfigDiagnostics() {
	path := uc.cfg.ConfigFailureStatePath
	if path == "" {
		path = filepath.Clean(uc.store.Path()) + ".diagnostic.json"
	}
	uc.failureStore = configstate.New(path, uc.store.Path(), uc.cfg.OutboxPath)
	state, err := uc.failureStore.Load()
	uc.failureState = state
	for _, item := range state.Pending {
		uc.attemptedReports[item.Diagnostic.ReportID] = true
	}
	uc.durable = err == nil
	if err != nil {
		uc.log.Info(logger.KV("config diagnostic durability unavailable", "version", 1))
	}
	if current, ok := state.Current(); ok {
		uc.applyDigest = current.IdentityDigest
		until := time.UnixMilli(current.Diagnostic.NextRetry)
		// A future/skewed clock cannot indefinitely suppress re-evaluation after restart.
		uc.applyUntil = minTime(until, uc.now().Add(retry.DefaultMaxBackoff))
	}
}

func (uc *ConfigSync) persistConfigDiagnostics() {
	uc.pruneAttemptedReports()
	if uc.now().Before(uc.persistAfter) {
		return
	}
	if err := uc.failureStore.Save(uc.failureState); err != nil {
		uc.durable = false
		uc.persistAfter = uc.now().Add(time.Minute)
		for i := range uc.failureState.Pending {
			if !uc.attemptedReports[uc.failureState.Pending[i].Diagnostic.ReportID] {
				uc.failureState.Pending[i].Diagnostic.Durable = false
			}
		}
		uc.log.Info(logger.KV("config diagnostic memory fallback", "version", 1, "pending", len(uc.failureState.Pending)))
		return
	}
	uc.durable = true
}

func (uc *ConfigSync) flushConfigDiagnostics(ctx context.Context) {
	if len(uc.failureState.Pending) == 0 {
		return
	}
	if _, active := uc.reportBackoff.Active(); active {
		return
	}
	bounded, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	for sent := 0; sent < 2 && len(uc.failureState.Pending) > 0; sent++ {
		item := uc.failureState.Pending[0]
		status, message := "apply_failed", configDiagnosticMessage(item.Diagnostic.Code)
		if item.Diagnostic.Phase == "recovered" {
			status = "applied"
			message = "configuration apply recovered; collection health not assessed"
		}
		uc.attemptedReports[item.Diagnostic.ReportID] = true
		started := time.Now()
		err := uc.reportConfigApply(bounded, item.Version, item.ConfigHash, status, message, item.Diagnostic)
		uc.log.Info(logger.KV("config diagnostic delivery", "version", item.Version, "phase", item.Diagnostic.Phase, "duration_ms", time.Since(started).Milliseconds(), "pending", len(uc.failureState.Pending)))
		if err != nil {
			if _, kind := uc.reportBackoff.Failure(err); kind != retry.ErrorKindTransient {
				uc.reportBackoff.Cooldown(retry.DefaultMaxBackoff)
			}
			return
		}
		uc.reportBackoff.Reset()
		uc.failureState.Acknowledge(item.Diagnostic.ReportID)
		delete(uc.attemptedReports, item.Diagnostic.ReportID)
		uc.persistConfigDiagnostics()
	}
}

func configDiagnosticMessage(code string) string {
	return fmt.Sprintf("configuration apply failed (%s); inspect local configuration", code)
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (uc *ConfigSync) pruneAttemptedReports() {
	pending := make(map[string]bool, len(uc.failureState.Pending))
	for _, item := range uc.failureState.Pending {
		pending[item.Diagnostic.ReportID] = true
	}
	for id := range uc.attemptedReports {
		if !pending[id] {
			delete(uc.attemptedReports, id)
		}
	}
}
