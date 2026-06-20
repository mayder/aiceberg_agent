package logdiscovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestCollectEmitsContractWithDiscoveredLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "error.log")
	if err := os.WriteFile(logPath, []byte("2026-06-20 error failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newCollector(config.Config{
		LogDiscoveryEnabled:          true,
		LogDiscoveryInterval:         time.Minute,
		LogDiscoveryMaxCandidates:    20,
		LogDiscoveryMaxEvidenceBytes: 512,
	}, func() config.CollectPrefs {
		return config.CollectPrefs{LogDiscoveryEnabled: true}
	}, []knownPath{{
		Path:                logPath,
		Kind:                "log_file",
		Product:             "nginx",
		ServiceName:         "nginx",
		RecommendedCategory: "observability",
		SOCSourceType:       "none",
		SOCEligible:         "conditional",
		Permissions:         []string{"read:test"},
	}})
	c.now = func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }
	c.hostname = func() (string, error) { return "test-host", nil }

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	body := decoded[schemaVersion]
	if body.SchemaVersion != schemaVersion {
		t.Fatalf("schema version mismatch: %#v", body.SchemaVersion)
	}
	if body.Host != "test-host" || body.ScanPolicy["default_min_severity"] != "error" {
		t.Fatalf("unexpected host/policy: %#v", body)
	}
	if len(body.Candidates) == 0 {
		t.Fatalf("expected candidates: %#v", body)
	}
	got := body.Candidates[0]
	if got.Fingerprint == "" || got.Path != logPath || got.MinSeverity != "error" {
		t.Fatalf("invalid candidate: %#v", got)
	}
	if got.SOCEligible != "conditional" || got.RedactionPolicy == "" {
		t.Fatalf("missing governance fields: %#v", got)
	}
}

func TestCollectDisabledReturnsNil(t *testing.T) {
	c := newCollector(config.Config{LogDiscoveryEnabled: true}, func() config.CollectPrefs {
		return config.CollectPrefs{Version: "remote", LogDiscoveryEnabled: false}
	}, nil)
	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if raw != nil {
		t.Fatalf("expected nil payload when disabled, got %s", string(raw))
	}
}

func TestCollectDetectsContainerKubernetesAndOTLPRuntimeSignals(t *testing.T) {
	dir := t.TempDir()
	dockerSock := filepath.Join(dir, "docker.sock")
	containerdSock := filepath.Join(dir, "containerd.sock")
	kubeToken := filepath.Join(dir, "token")
	for _, path := range []string{dockerSock, containerdSock, kubeToken} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	c := newCollector(config.Config{
		LogDiscoveryEnabled:       true,
		ContainerDockerSocket:     dockerSock,
		ContainerContainerdSocket: containerdSock,
		KubernetesTokenPath:       kubeToken,
		OTLPEnabled:               true,
		OTLPHTTPAddr:              "127.0.0.1:4318",
	}, nil, nil)
	c.now = func() time.Time { return time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC) }
	c.hostname = func() (string, error) { return "runtime-host", nil }

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	body := decoded[schemaVersion]
	assertCandidateProduct(t, body.Candidates, "docker")
	assertCandidateProduct(t, body.Candidates, "containerd")
	assertCandidateProduct(t, body.Candidates, "kubernetes")
	assertCandidateProduct(t, body.Candidates, "opentelemetry")
	if body.Capabilities["docker_socket"] != true || body.Capabilities["kubernetes_token"] != true || body.Capabilities["otlp_enabled"] != true {
		t.Fatalf("unexpected capabilities: %#v", body.Capabilities)
	}
}

func TestFingerprintDeduplicatesSuperficialDuplicates(t *testing.T) {
	row := baseCandidate("log_file", "nginx")
	row.Path = "/var/log/nginx/error.log"
	row.ServiceName = "nginx"
	row.Fingerprint = fingerprint(row)

	dup := row
	dup.Evidence = []string{"size:10", "mtime:changed"}
	dup.Fingerprint = fingerprint(dup)

	c := newCollector(config.Config{}, nil, nil)
	rows := c.deduplicate([]Candidate{row, dup}, 10)
	if len(rows) != 1 {
		t.Fatalf("expected one deduped candidate, got %d", len(rows))
	}
}

func TestSanitizeTextRedactsSecretLikeArguments(t *testing.T) {
	got := sanitizeText("worker --token abc --password=secret --safe ok", 200)
	if got == "" || got == "worker --token abc --password=secret --safe ok" {
		t.Fatalf("expected redaction, got %q", got)
	}
	if got != "worker [redacted] [redacted] [redacted] --safe ok" {
		t.Fatalf("unexpected redacted value: %q", got)
	}
}

func assertCandidateProduct(t *testing.T, rows []Candidate, product string) {
	t.Helper()
	for _, row := range rows {
		if row.Product == product {
			if row.Fingerprint == "" || row.MinSeverity != "error" || row.RedactionPolicy == "" {
				t.Fatalf("candidate %s missing governance fields: %#v", product, row)
			}
			return
		}
	}
	t.Fatalf("candidate product %q not found in %#v", product, rows)
}
