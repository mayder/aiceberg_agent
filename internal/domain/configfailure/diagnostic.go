// Package configfailure defines bounded, secret-free configuration failure facts.
package configfailure

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"strconv"
	"syscall"
)

const SchemaVersion = 1
const MaxCount int64 = 2147483647

// Report is additive to the legacy config-report status/version/hash fields.
// Timestamps are UTC Unix milliseconds; zero NextRetry means no retry is scheduled.
type Report struct {
	SchemaVersion int    `json:"schema_version"`
	ReportID      string `json:"report_id"`
	Fingerprint   string `json:"fingerprint"`
	Code          string `json:"code"`
	Kind          string `json:"kind"`
	Phase         string `json:"phase"`
	FirstSeen     int64  `json:"first_seen_ms"`
	LastSeen      int64  `json:"last_seen_ms"`
	Count         int64  `json:"count"`
	Attempt       int64  `json:"attempt"`
	NextRetry     int64  `json:"next_retry_ms"`
	Durable       bool   `json:"durable"`
}

type Cause struct{ Code, Kind string }

// Classify relies on typed errors, never on messages that may contain private paths.
func Classify(err error) Cause {
	if cause, ok := platformCause(err); ok {
		return cause
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return Cause{"permission_denied", "persistent"}
	case errors.Is(err, fs.ErrExist), errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EISDIR), errors.Is(err, syscall.ELOOP):
		return Cause{"path_conflict", "persistent"}
	case errors.Is(err, syscall.ENOSPC):
		return Cause{"disk_full", "persistent"}
	case errors.Is(err, syscall.EROFS):
		return Cause{"read_only", "persistent"}
	case errors.Is(err, syscall.EBUSY), errors.Is(err, syscall.EAGAIN):
		return Cause{"concurrent_change", "transient"}
	case errors.Is(err, syscall.EINTR), errors.Is(err, syscall.ETIMEDOUT):
		return Cause{"io_temporary", "transient"}
	default:
		return Cause{"unknown", "unknown"}
	}
}

// Fingerprint includes authenticated agent identity, operation, config digest and cause.
// Length prefixes avoid ambiguous concatenation; raw values never leave this function.
func Fingerprint(agent, operation, configDigest, code string) string {
	h := sha256.New()
	for _, value := range []string{agent, operation, configDigest, code} {
		_, _ = h.Write([]byte(strconv.Itoa(len(value)) + ":" + value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r Report) ID() string {
	return Fingerprint(r.Fingerprint, r.Phase, strconv.FormatInt(r.LastSeen, 10), strconv.FormatInt(r.Count, 10))
}
