package metrics

import "sync/atomic"

var configApplyAttempts atomic.Int64
var configApplySuppressed atomic.Int64
var configReportBytes atomic.Int64
var configReportDurationMS atomic.Int64
var configDiagnosticDropped atomic.Int64

func ConfigApplyAttempt()    { configApplyAttempts.Add(1) }
func ConfigApplySuppressed() { configApplySuppressed.Add(1) }
func ConfigReportSent(bytes, durationMS int64) {
	configReportBytes.Add(bytes)
	configReportDurationMS.Add(durationMS)
}
func ConfigDiagnosticDropped(count int64) {
	if count > 0 {
		configDiagnosticDropped.Add(count)
	}
}

// ConfigFailureSnapshot has no identity, path or fingerprint labels.
func ConfigFailureSnapshot() map[string]int64 {
	return map[string]int64{
		"attempts": configApplyAttempts.Load(), "suppressed": configApplySuppressed.Load(),
		"report_bytes": configReportBytes.Load(), "report_duration_ms": configReportDurationMS.Load(),
		"dropped_reports": configDiagnosticDropped.Load(),
	}
}
