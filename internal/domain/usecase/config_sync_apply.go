package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/you/aiceberg_agent/internal/common/logger"
	"github.com/you/aiceberg_agent/internal/common/metrics"
	"github.com/you/aiceberg_agent/internal/common/retry"
	"github.com/you/aiceberg_agent/internal/domain/configfailure"
)

// ApplyIncomingConfig handles own ingest config with the same policy as polling.
// It performs no network I/O; queued diagnostics are delivered by normal ConfigSync.
func (uc *ConfigSync) ApplyIncomingConfig(payload ConfigPayload) (string, bool, error) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	raw, _ := json.Marshal(payload)
	version, applied, err := uc.applyConfig(payload, hashPayload(raw))
	uc.RequestFullSync()
	return version, applied, err
}

func (uc *ConfigSync) applyConfig(payload ConfigPayload, rawHash string) (string, bool, error) {
	digest := uc.configApplyDigest(payload)
	now := uc.now()
	if uc.suppressConfigApply(digest, payload.Version, now) {
		return "", false, nil
	}
	if uc.applyDigest != digest {
		uc.applyBackoff.Reset()
	}
	metrics.ConfigApplyAttempt()
	started := time.Now()
	version, applied, err := ApplyConfigPayloadWithSecurity(uc.log, uc.store, uc.commands, payload, ConfigSecurityOptions{
		SignatureSecret: uc.cfg.RemoteConfigSignatureSecret, SignatureRequired: uc.cfg.RemoteConfigSignatureRequired,
		AllowUnsignedSensitive: uc.cfg.RemoteConfigAllowUnsignedSensitive,
	})
	uc.log.Info(logger.KV("config apply attempt", "version", version, "duration_ms", time.Since(started).Milliseconds(), "ok", err == nil))
	if err != nil {
		uc.recordConfigApplyFailure(version, digest, rawHash, err, now)
		return version, false, fmt.Errorf("configuration apply failed (%s)", configfailure.Classify(err).Code)
	}
	uc.confirmConfigApply(version, rawHash, now)
	return version, applied, nil
}

func (uc *ConfigSync) recordConfigApplyFailure(version, digest, rawHash string, err error, now time.Time) {
	cause := configfailure.Classify(err)
	delay := retry.DefaultMaxBackoff
	if cause.Kind == "transient" {
		// Reuse the existing capped jitter policy, classifying typed local transient errors.
		delay, _ = uc.applyBackoff.Failure(context.DeadlineExceeded)
	}
	uc.applyDigest = digest
	uc.applyUntil = now.Add(delay)
	agent := fmt.Sprint(uc.cfg.AgentID)
	if uc.cfg.AgentID == 0 {
		agent = hashPayload([]byte(uc.cfg.Agent.Token))
	}
	before := uc.failureState.Dropped
	uc.failureState.Failure(agent, version, digest, rawHash, cause, now.UnixMilli(), uc.applyUntil.UnixMilli(), uc.durable)
	metrics.ConfigDiagnosticDropped(uc.failureState.Dropped - before)
	uc.persistConfigDiagnostics()
	uc.log.Info(logger.KV("config apply deferred", "version", version, "cause", cause.Code, "retry_after_ms", delay.Milliseconds(), "durable", uc.durable))
}

// Signature timestamps/value are volatile; validity is material to retry admission.
// A newly valid signature must not inherit an earlier rejected payload's wait.
func (uc *ConfigSync) configApplyDigest(payload ConfigPayload) string {
	identity := payload
	identity.Signature = PayloadSignature{}
	raw, _ := json.Marshal(identity)
	validity := "valid"
	if ValidateConfigPayloadSecurity(payload, ConfigSecurityOptions{
		SignatureSecret: uc.cfg.RemoteConfigSignatureSecret, SignatureRequired: uc.cfg.RemoteConfigSignatureRequired,
		AllowUnsignedSensitive: uc.cfg.RemoteConfigAllowUnsignedSensitive,
	}) != nil {
		validity = "rejected"
	}
	return configfailure.Fingerprint("", "apply-config", hashPayload(raw), validity)
}

func (uc *ConfigSync) suppressConfigApply(digest, version string, now time.Time) bool {
	if uc.applyDigest == digest && now.Before(uc.applyUntil) {
		metrics.ConfigApplySuppressed()
		before := uc.failureState.Dropped
		uc.failureState.Repeated(now.UnixMilli())
		metrics.ConfigDiagnosticDropped(uc.failureState.Dropped - before)
		if !now.Before(uc.checkpointAfter) {
			uc.persistConfigDiagnostics()
			uc.checkpointAfter = now.Add(time.Minute)
		}
		uc.log.Info(logger.KV("config apply suppressed", "version", version, "retry_after_ms", uc.applyUntil.Sub(now).Milliseconds()))
		return true
	}
	return false
}

func (uc *ConfigSync) confirmConfigApply(version, rawHash string, now time.Time) {
	uc.applyBackoff.Reset()
	uc.applyUntil = time.Time{}
	uc.applyDigest = ""
	if current, ok := uc.failureState.Current(); ok {
		// The recovery identifies the prior condition but confirms the actually applied version.
		for i := range uc.failureState.Entries {
			if uc.failureState.Entries[i].Diagnostic.Fingerprint == current.Diagnostic.Fingerprint {
				uc.failureState.Entries[i].Version = version
				uc.failureState.Entries[i].ConfigHash = rawHash
			}
		}
		uc.failureState.Recover(now.UnixMilli(), uc.durable)
		uc.persistConfigDiagnostics()
	}
}
