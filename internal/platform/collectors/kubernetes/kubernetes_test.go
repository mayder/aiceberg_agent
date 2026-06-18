package kubernetes

import (
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
		"aiceberg.ai/checks":    `[{"type":"http","url":"http://%%host%%:8080/health"}]`,
		"aiceberg.ai/check.tcp": "8080",
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
