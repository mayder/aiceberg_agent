package health

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestHealthSnapshotIncludesChannelStatus(t *testing.T) {
	snap := Snapshot{
		Status:  "ok",
		Version: "test",
		Channel: map[string]any{
			"mode":               "direct",
			"fallback_active":    true,
			"last_latency_ms":    25,
			"reconnect_retries":  2,
			"last_heartbeat_utc": "2026-04-26T12:00:00Z",
		},
	}

	rr := httptest.NewRecorder()
	encodeHealthSnapshot(rr, snap)

	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	channel, ok := got["channel"].(map[string]any)
	if !ok {
		t.Fatalf("expected channel status, got %#v", got)
	}
	if channel["mode"] != "direct" || channel["fallback_active"] != true {
		t.Fatalf("unexpected channel status %#v", channel)
	}
	if got["agent_pipeline_version"] != "2-compatible" {
		t.Fatalf("expected pipeline version, got %#v", got["agent_pipeline_version"])
	}
}
