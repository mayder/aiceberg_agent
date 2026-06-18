package localchecks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestRunHTTPCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "http",
		Target:  srv.URL,
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected http ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" || len(metrics) != 1 {
		t.Fatalf("unexpected result %#v %#v", serviceCheck, metrics)
	}
}

func TestRunOpenMetricsParsesMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("# HELP x\nrequests_total{code=\"200\"} 12\nlatency_seconds 1.5\n"))
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "openmetrics",
		Target:  srv.URL,
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected openmetrics ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" || len(metrics) != 2 {
		t.Fatalf("unexpected result %#v %#v", serviceCheck, metrics)
	}
	if metrics[0]["name"] != "requests_total" {
		t.Fatalf("expected metric name without labels, got %#v", metrics[0])
	}
}

func TestRunOpenMetricsAppliesAllowlistAndLabelLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http_requests_total{code=\"200\",pod=\"a\"} 12\nhttp_requests_total{code=\"500\",pod=\"b\"} 1\nsecret_metric{token=\"abc\"} 99\n"))
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "openmetrics",
		Target:  srv.URL,
		Enabled: true,
		Config: map[string]string{
			"metric_allowlist": "http_*",
			"label_allowlist":  "code",
			"max_label_values": "1",
		},
	}, 1024)
	if err != nil {
		t.Fatalf("expected openmetrics ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
	if len(metrics) != 2 {
		t.Fatalf("expected 2 allowed metrics, got %#v", metrics)
	}
	labels, ok := metrics[0]["labels"].(map[string]string)
	if !ok || labels["code"] != "200" {
		t.Fatalf("expected allowed code label, got %#v", metrics[0])
	}
	if _, hasLabels := metrics[1]["labels"]; hasLabels {
		t.Fatalf("expected second metric labels dropped by cardinality limit, got %#v", metrics[1])
	}
}

func TestRunJMXJolokiaParsesSafeMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":200,"value":{"HeapMemoryUsage":{"used":128,"max":512},"ThreadCount":24,"CollectionCount":7,"CollectionTime":42}}`))
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "jmx",
		Target:  srv.URL,
		Enabled: true,
		Config: map[string]string{
			"mode":                "jolokia",
			"homologation_status": "approved",
			"homologation_ref":    "pkg71-local-fixture",
		},
	}, 1024)
	if err != nil {
		t.Fatalf("expected jmx ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
	if len(metrics) < 3 {
		t.Fatalf("expected jvm metrics, got %#v", metrics)
	}
}

func TestBetaIntegrationRequiresHomologation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("beta integration should be blocked before reaching the target")
	}))
	defer srv.Close()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "jmx",
		Target:  srv.URL,
		Enabled: true,
		Config:  map[string]string{"mode": "jolokia"},
	}, 1024)
	if err == nil {
		t.Fatal("expected homologation gate error")
	}
	if len(metrics) != 0 {
		t.Fatalf("expected no metrics, got %#v", metrics)
	}
	if serviceCheck["status"] != "blocked" || serviceCheck["reason"] != "integration_not_homologated" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
	if manifest, ok := serviceCheck["integration"].(map[string]any); !ok || manifest["status"] != "beta" {
		t.Fatalf("expected beta integration metadata, got %#v", serviceCheck)
	}
}

func TestRunTCPCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	_, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "redis",
		Target:  ln.Addr().String(),
		Enabled: true,
	}, 1024)
	if err != nil {
		t.Fatalf("expected tcp ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
	if manifest, ok := serviceCheck["integration"].(map[string]any); !ok || manifest["status"] != "official" {
		t.Fatalf("expected official integration metadata, got %#v", serviceCheck)
	}
}

func TestBetaDatabaseAndQueueRequireHomologation(t *testing.T) {
	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "sqlserver",
		Target:  "127.0.0.1:1433",
		Enabled: true,
	}, 1024)
	if err == nil {
		t.Fatal("expected homologation gate error")
	}
	if len(metrics) != 0 {
		t.Fatalf("expected no metrics, got %#v", metrics)
	}
	if serviceCheck["status"] != "blocked" || serviceCheck["reason"] != "integration_not_homologated" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
	if manifest, ok := serviceCheck["integration"].(map[string]any); !ok || manifest["kind"] != "sqlserver" || manifest["status"] != "beta" {
		t.Fatalf("expected beta sqlserver metadata, got %#v", serviceCheck)
	}
}

func TestRabbitMQHomologatedReachability(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	metrics, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "rabbitmq",
		Target:  ln.Addr().String(),
		Enabled: true,
		Config: map[string]string{
			"homologation_status": "approved",
			"homologation_ref":    "pkg71-rabbitmq-fixture",
		},
	}, 1024)
	if err != nil {
		t.Fatalf("expected rabbitmq tcp ok, got %v", err)
	}
	if serviceCheck["status"] != "ok" || len(metrics) != 1 || metrics[0]["integration"] != "rabbitmq" {
		t.Fatalf("unexpected rabbitmq result %#v %#v", serviceCheck, metrics)
	}
}

func TestWindowsIntegrationSkipsOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows guard test")
	}
	_, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "iis_wmi",
		Enabled: true,
		Config: map[string]string{
			"homologation_status": "approved",
			"homologation_ref":    "pkg71-windows-contract",
		},
	}, 1024)
	if err == nil {
		t.Fatal("expected windows_only error outside Windows")
	}
	if serviceCheck["status"] != "skipped" || serviceCheck["reason"] != "windows_only" {
		t.Fatalf("unexpected windows guard service check %#v", serviceCheck)
	}
	if manifest, ok := serviceCheck["integration"].(map[string]any); !ok || manifest["status"] != "experimental" {
		t.Fatalf("expected experimental integration metadata, got %#v", serviceCheck)
	}
}

func TestWindowsServiceRejectsUnsafeServiceName(t *testing.T) {
	_, _, serviceCheck, err := execute(context.Background(), config.LocalCheckConfig{
		Kind:    "windows_service",
		Enabled: true,
		Target:  "Spooler;Remove-Item",
		Config: map[string]string{
			"homologation_status": "approved",
			"homologation_ref":    "pkg71-windows-contract",
		},
	}, 1024)
	if err == nil {
		t.Fatal("expected unsafe service name error")
	}
	if serviceCheck["status"] != "critical" {
		t.Fatalf("unexpected service check %#v", serviceCheck)
	}
}

func TestDisallowsArbitraryCheckKindAndRedactsResult(t *testing.T) {
	c := &collector{baseEnabled: true, baseChecks: []config.LocalCheckConfig{{
		ID:             "bad",
		Kind:           "shell",
		Target:         "token=abc",
		CredentialsRef: "vault/path",
		Enabled:        true,
	}}, interval: time.Second, maxChecks: 10, maxBytes: 1024}

	raw, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !contains(body, "[redacted]") || contains(body, "abc") || contains(body, "vault/path") {
		t.Fatalf("expected redacted payload, got %s", body)
	}
}

func TestInstalledIntegrationsLoadsSafeManifestsOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openmetrics.json"), []byte(`{
		"schema_version":1,
		"kind":"openmetrics",
		"version":"2",
		"status":"official",
		"owner":"aiceberg_agent",
		"permissions":["http_get"],
		"rollback":"disable check"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{
		"schema_version":1,
		"kind":"redis",
		"version":"1",
		"status":"official",
		"permissions":["shell_exec"]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	installed := installedIntegrations([]string{dir})
	if len(installed) != 1 {
		t.Fatalf("expected one safe manifest, got %#v", installed)
	}
	if installed[0]["kind"] != "openmetrics" || installed[0]["status"] != "official" {
		t.Fatalf("unexpected manifest payload %#v", installed[0])
	}
}

func contains(value, part string) bool {
	return len(part) == 0 || (len(value) >= len(part) && index(value, part) >= 0)
}

func index(value, part string) int {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return i
		}
	}
	return -1
}
