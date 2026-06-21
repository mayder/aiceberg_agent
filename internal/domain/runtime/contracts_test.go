package runtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWithPayloadMetadataAddsCompatibleFields(t *testing.T) {
	raw := []byte(`{"cpu":{"percent_total":12.5}}`)

	got, err := WithPayloadMetadata(raw, "sysmetrics", "/v1/ingest/metrics")
	if err != nil {
		t.Fatalf("WithPayloadMetadata() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(got, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["cpu"].(map[string]any); !ok {
		t.Fatalf("expected original cpu section, got %#v", body)
	}
	if body["schema_version"] != float64(PayloadSchemaV1) {
		t.Fatalf("unexpected schema_version %#v", body["schema_version"])
	}
	if body["agent_pipeline_version"] != PipelineVersion {
		t.Fatalf("unexpected agent_pipeline_version %#v", body["agent_pipeline_version"])
	}
	if body["collector_name"] != "sysmetrics" || body["ingest_endpoint"] != "/v1/ingest/metrics" {
		t.Fatalf("unexpected metadata %#v", body)
	}
	host, ok := body["host"].(map[string]any)
	if !ok || host["hostname"] == "" || host["os"] == "" || host["arch"] == "" {
		t.Fatalf("expected host metadata, got %#v", body["host"])
	}
}

func TestWithPayloadMetadataRejectsInvalidJSON(t *testing.T) {
	if _, err := WithPayloadMetadata([]byte(`not-json`), "sysmetrics", "/v1/ingest/metrics"); err == nil {
		t.Fatalf("expected invalid json error")
	}
}

func TestSchedulerSnapshotForCollectorsIsStableCopy(t *testing.T) {
	collectors := []CollectorSpec{{
		Name:     "sysmetrics",
		Version:  "legacy-compatible",
		Endpoint: "/v1/ingest/metrics",
		Interval: 10 * time.Second,
	}}

	snap := SchedulerSnapshotForCollectors(collectors)
	collectors[0].Name = "mutated"

	if snap.PipelineVersion != PipelineVersion {
		t.Fatalf("unexpected pipeline version %q", snap.PipelineVersion)
	}
	if snap.Collectors[0].Name != "sysmetrics" {
		t.Fatalf("snapshot should not track caller slice mutation: %#v", snap.Collectors)
	}
}
