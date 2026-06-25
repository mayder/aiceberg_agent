package app

import "testing"

func TestWorkerErrorFingerprintUsesStableContext(t *testing.T) {
	first := workerErrorFingerprint("collect_oslogs_failed", "direct", map[string]any{
		"route": "/v1/logs/raw",
	})
	second := workerErrorFingerprint("collect_oslogs_failed", "direct", map[string]any{
		"route":   "/v1/logs/raw",
		"summary": "oslogs: transient message changed",
	})
	if first != second {
		t.Fatalf("expected stable fingerprint for same error context")
	}

	otherRoute := workerErrorFingerprint("collect_oslogs_failed", "direct", map[string]any{
		"route": "/v1/ingest",
	})
	if first == otherRoute {
		t.Fatalf("expected different fingerprint for different route")
	}
}
