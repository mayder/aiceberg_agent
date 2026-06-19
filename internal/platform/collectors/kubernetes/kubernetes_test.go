package kubernetes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNormalizePodsRedactsSensitiveAnnotationsAndAddsContainerStatus(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "api-123"
	p.Metadata.Namespace = "prod"
	p.Metadata.UID = "uid-1"
	p.Metadata.Labels = map[string]string{"app": "api", "password": "clear"}
	p.Metadata.Annotations = map[string]string{
		"aiceberg.ai/check.http": "http://%%host%%:8080/health",
		"token":                  "secret",
	}
	p.Spec.NodeName = "node-a"
	p.Spec.Containers = []containerSpec{{Name: "api", Image: "api:1"}}
	p.Status.Phase = "Running"
	p.Status.ContainerStatuses = []containerStatus{{
		Name:         "api",
		Ready:        true,
		RestartCount: 2,
		ContainerID:  "containerd://abcdef1234567890",
	}}

	out := normalizePods([]pod{p})
	if len(out) != 1 {
		t.Fatalf("expected one pod, got %#v", out)
	}
	labels := out[0]["labels"].(map[string]string)
	if labels["password"] != "[redacted]" {
		t.Fatalf("expected redacted label, got %#v", labels)
	}
	annotations := out[0]["annotations"].(map[string]string)
	if annotations["token"] != "[redacted]" {
		t.Fatalf("expected redacted annotation, got %#v", annotations)
	}
	containers := out[0]["containers"].([]map[string]any)
	if containers[0]["restart_count"] != 2 || containers[0]["container_id"] != "abcdef123456" {
		t.Fatalf("expected status mapped, got %#v", containers[0])
	}
}

func TestAutodiscoveryChecksFromAnnotations(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "web"
	p.Metadata.Namespace = "default"
	p.Metadata.Annotations = map[string]string{
		"aiceberg.ai/checks":          `[{"type":"http","url":"http://%%host%%:8080/health"}]`,
		"aiceberg.ai/check.tcp":       "8080",
		"aiceberg.ai/tool-origin":     "application",
		"aiceberg.ai/source-category": "conditional",
		"aiceberg.ai/soc-source-type": "application",
		"aiceberg.ai/soc-eligible":    "conditional",
		"aiceberg.ai/route-reason":    "kubernetes_annotation",
	}

	checks := autodiscoveryChecks([]pod{p})
	if len(checks) != 2 {
		t.Fatalf("expected two checks, got %#v", checks)
	}
	if checks[0]["namespace"] != "default" || checks[0]["pod"] != "web" {
		t.Fatalf("expected pod identity on JSON check, got %#v", checks[0])
	}
	if checks[0]["kind"] != "http" || checks[0]["target"] != "http://web:8080/health" || checks[0]["enabled"] != true {
		t.Fatalf("expected canonical executable http check, got %#v", checks[0])
	}
	if checks[1]["key"] != "tcp" || checks[1]["value"] != "8080" {
		t.Fatalf("expected simple annotation check, got %#v", checks[1])
	}
	if checks[1]["kind"] != "tcp" || checks[1]["target"] != "web:8080" || checks[1]["enabled"] != true {
		t.Fatalf("expected canonical executable tcp check, got %#v", checks[1])
	}
}

func TestParsePodLogLineAddsTagsAndRedactsSecrets(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "api-123"
	p.Metadata.Namespace = "prod"
	p.Metadata.UID = "uid-1"
	p.Spec.NodeName = "node-a"
	spec := containerSpec{Name: "api", Image: "api:1"}

	event, timestamp, ok := parsePodLogLine("2026-06-18T10:00:00Z started password=secret", p, spec, 1024)
	if !ok {
		t.Fatalf("expected log parsed")
	}
	if timestamp != "2026-06-18T10:00:00Z" || event["timestamp_utc"] != timestamp {
		t.Fatalf("expected timestamp preserved, got event=%#v timestamp=%q", event, timestamp)
	}
	if event["namespace"] != "prod" || event["pod"] != "api-123" || event["container"] != "api" || event["node_name"] != "node-a" {
		t.Fatalf("expected Kubernetes tags, got %#v", event)
	}
	if strings.Contains(event["message"].(string), "secret") || event["redaction_status"] != "redacted" {
		t.Fatalf("expected redacted message, got %#v", event)
	}
}

func TestReadPodLogStreamUpdatesCursor(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "api-123"
	p.Metadata.Namespace = "prod"
	spec := containerSpec{Name: "api", Image: "api:1"}
	cursor := map[string]string{}
	key := podLogCursorKey(p, spec)

	events, dropped := readPodLogStream(strings.NewReader("2026-06-18T10:00:00Z first\n2026-06-18T10:00:01Z second\n"), p, spec, cursor, key, 10, 1024)
	if dropped != 0 || len(events) != 2 {
		t.Fatalf("expected two events, got events=%#v dropped=%d", events, dropped)
	}
	if cursor[key] != "2026-06-18T10:00:01Z" {
		t.Fatalf("expected latest cursor timestamp, got %q", cursor[key])
	}
}

func TestMatchesLogFilterByNamespacePodContainerImageAndLabels(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "api-123"
	p.Metadata.Namespace = "prod"
	p.Metadata.Labels = map[string]string{"app": "api", "tier": "backend"}
	p.Spec.NodeName = "node-a"
	spec := containerSpec{Name: "api", Image: "api:1"}

	if !matchesLogFilter(p, spec, compileRegex("prod|backend|api:1"), nil) {
		t.Fatalf("expected include filter to match")
	}
	if matchesLogFilter(p, spec, nil, compileRegex("node-a|backend")) {
		t.Fatalf("expected exclude filter to reject")
	}
}

func TestPKG65KubernetesPayloadAutodiscoveryMetricsEvidence(t *testing.T) {
	p := pod{}
	p.Metadata.Name = "api-7d9"
	p.Metadata.Namespace = "prod"
	p.Metadata.UID = "pod-uid-1"
	p.Metadata.Labels = map[string]string{"app": "api", "tier": "backend"}
	p.Metadata.Annotations = map[string]string{
		"aiceberg.ai/checks":          `[{"type":"http","url":"http://%%host%%:8080/health"}]`,
		"aiceberg.ai/check.tcp":       "8080",
		"aiceberg.ai/tool-origin":     "application",
		"aiceberg.ai/source-category": "conditional",
		"aiceberg.ai/soc-source-type": "application",
		"aiceberg.ai/soc-eligible":    "conditional",
		"aiceberg.ai/route-reason":    "kubernetes_annotation",
	}
	p.Spec.NodeName = "node-a"
	p.Spec.Containers = []containerSpec{{Name: "api", Image: "api:1"}}
	p.Spec.Volumes = []map[string]any{{"name": "sensitive-volume", "secret": map[string]any{"secretName": "do-not-collect"}}}
	p.Status.Phase = "Running"
	p.Status.PodIP = "10.42.0.15"
	p.Status.ContainerStatuses = []containerStatus{{
		Name:         "api",
		Ready:        true,
		RestartCount: 1,
		ContainerID:  "containerd://abcdef1234567890",
		State:        map[string]any{"running": map[string]any{"startedAt": "2026-06-19T18:00:00Z"}},
	}}

	n := node{}
	n.Metadata.Name = "node-a"
	n.Metadata.UID = "node-uid-1"
	n.Metadata.Labels = map[string]string{"kubernetes.io/os": "linux"}
	n.Status.Capacity = map[string]string{"cpu": "4", "memory": "8Gi"}
	n.Status.Allocatable = map[string]string{"cpu": "3900m", "memory": "7Gi"}

	ev := event{}
	ev.Metadata.Name = "api-started"
	ev.Metadata.Namespace = "prod"
	ev.Type = "Normal"
	ev.Reason = "Started"
	ev.Message = "Started container api"
	ev.InvolvedObject.Kind = "Pod"
	ev.InvolvedObject.Namespace = "prod"
	ev.InvolvedObject.Name = "api-7d9"
	ev.Count = 1

	spec := p.Spec.Containers[0]
	cursor := map[string]string{}
	logs, dropped := readPodLogStream(strings.NewReader("2026-06-19T18:00:00Z started password=SHOULD_NOT_LEAK\n"), p, spec, cursor, podLogCursorKey(p, spec), 10, 1024)
	payload := map[string]any{
		"kubernetes": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "kubernetes_api_controlled",
			"node_name":            "node-a",
			"namespace_scope":      "prod",
			"pods":                 normalizePods([]pod{p}),
			"nodes":                normalizeNodes([]node{n}),
			"events":               normalizeEvents([]event{ev}),
			"metrics":              map[string]any{"nodes": normalizeNodeMetrics([]nodeMetric{testNodeMetric()}), "pods": normalizePodMetrics([]podMetric{testPodMetric()})},
			"logs":                 map[string]any{"schema_version": schemaVersion, "events": logs, "dropped_count": dropped},
			"autodiscovery_checks": autodiscoveryChecks([]pod{p}),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(raw), "SHOULD_NOT_LEAK") || strings.Contains(string(raw), "do-not-collect") || strings.Contains(string(raw), "sensitive-volume") {
		t.Fatalf("payload leaked sensitive log or volume: %s", string(raw))
	}
	k8s := payload["kubernetes"].(map[string]any)
	if len(k8s["pods"].([]map[string]any)) != 1 || len(k8s["nodes"].([]map[string]any)) != 1 || len(k8s["events"].([]map[string]any)) != 1 {
		t.Fatalf("expected one pod, node and event, got %#v", k8s)
	}
	if len(k8s["autodiscovery_checks"].([]map[string]any)) != 2 {
		t.Fatalf("expected two autodiscovery checks, got %#v", k8s["autodiscovery_checks"])
	}
	metrics := k8s["metrics"].(map[string]any)
	if len(metrics["nodes"].([]map[string]any)) != 1 || len(metrics["pods"].([]map[string]any)) != 1 {
		t.Fatalf("expected node and pod metrics, got %#v", metrics)
	}
	logEvents := k8s["logs"].(map[string]any)["events"].([]map[string]any)
	if len(logEvents) != 1 || logEvents[0]["redaction_status"] != "redacted" {
		t.Fatalf("expected redacted pod log, got %#v", logEvents)
	}
	if logEvents[0]["aiceberg_tool_origin"] != "application" || logEvents[0]["aiceberg_source_category"] != "conditional" || logEvents[0]["aiceberg_soc_source_type"] != "application" || logEvents[0]["aiceberg_origin_confidence"] != "configured" {
		t.Fatalf("expected Kubernetes log SOC contract, got %#v", logEvents[0])
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG65_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG65Evidence(t, evidenceDir, raw, map[string]string{
			"pods_seen":             "1",
			"nodes_seen":            "1",
			"events_seen":           "1",
			"pod_logs_seen":         strconv.Itoa(len(logEvents)),
			"autodiscovery_checks":  "2",
			"node_metrics_seen":     "1",
			"pod_metrics_seen":      "1",
			"secret_volume_present": "yes",
			"secret_volume_leaked":  "no",
			"log_redaction":         "yes",
			"kind_real_reference":   "docs/evidence/pkg69/kubernetes-rbac-20260619T041959Z/evidence.md",
		})
	}
}

func testNodeMetric() nodeMetric {
	row := nodeMetric{}
	row.Metadata.Name = "node-a"
	row.Timestamp = "2026-06-19T18:00:00Z"
	row.Window = "30s"
	row.Usage = map[string]string{"cpu": "120m", "memory": "512Mi"}
	return row
}

func testPodMetric() podMetric {
	row := podMetric{}
	row.Metadata.Name = "api-7d9"
	row.Metadata.Namespace = "prod"
	row.Timestamp = "2026-06-19T18:00:00Z"
	row.Window = "30s"
	row.Containers = []containerMetricUsage{{Name: "api", Usage: map[string]string{"cpu": "40m", "memory": "128Mi"}}}
	return row
}

func writePKG65Evidence(t *testing.T, dir string, payload []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kubernetes_payload.json"), payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	keys := []string{"pods_seen", "nodes_seen", "events_seen", "pod_logs_seen", "autodiscovery_checks", "node_metrics_seen", "pod_metrics_seen", "secret_volume_present", "secret_volume_leaked", "log_redaction", "kind_real_reference"}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}
