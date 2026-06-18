package kubernetes

import "testing"

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
	if checks[1]["key"] != "tcp" || checks[1]["value"] != "8080" {
		t.Fatalf("expected simple annotation check, got %#v", checks[1])
	}
}
