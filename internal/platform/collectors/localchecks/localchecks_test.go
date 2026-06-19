package localchecks

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

func TestGuideCreatedIntegrationManifestIsAccepted(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rabbitmq.json"), []byte(`{
		"schema_version":1,
		"kind":"rabbitmq",
		"version":"1",
		"status":"beta",
		"owner":"aiceberg_agent",
		"permissions":["tcp_connect","credentials_ref"],
		"rollback":"disable rabbitmq local check"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	installed := installedIntegrations([]string{dir})
	if len(installed) != 1 {
		t.Fatalf("expected guide manifest accepted, got %#v", installed)
	}
	if installed[0]["kind"] != "rabbitmq" || installed[0]["status"] != "beta" {
		t.Fatalf("unexpected guide manifest payload %#v", installed[0])
	}
}

func TestPKG66LocalChecksLifecycleRollbackUpgradeEvidence(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer okServer.Close()
	metricsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("local_requests_total{code=\"200\"} 7\n"))
	}))
	defer metricsServer.Close()

	manifestDir := t.TempDir()
	writeManifest(t, manifestDir, "openmetrics", "1")
	checks := []config.LocalCheckConfig{
		{ID: "api-ok", Kind: "http", Target: okServer.URL + "?token=SHOULD_NOT_LEAK", CredentialsRef: "vault/local/api", Enabled: true, TimeoutMs: 1000},
		{ID: "api-fail", Kind: "http", Target: "http://127.0.0.1:1/health?password=SHOULD_NOT_LEAK", Enabled: true, TimeoutMs: 50},
		{ID: "blocked-shell", Kind: "shell", Target: "echo unsafe", Enabled: true, TimeoutMs: 50},
		{ID: "metrics", Kind: "openmetrics", Target: metricsServer.URL, Enabled: true, TimeoutMs: 1000},
	}
	c := &collector{baseEnabled: true, baseChecks: checks, interval: time.Second, maxChecks: 10, maxBytes: 4096, manifestDirs: []string{manifestDir}}
	rawBefore, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect before upgrade: %v", err)
	}
	writeManifest(t, manifestDir, "openmetrics", "2")
	rawAfter, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect after upgrade: %v", err)
	}
	c.prefs = func() config.CollectPrefs {
		return config.CollectPrefs{Version: "rollback", LocalChecksEnabled: false}
	}
	rawRollback, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect rollback: %v", err)
	}
	if rawRollback != nil {
		t.Fatalf("expected rollback disabled collector, got %s", string(rawRollback))
	}
	if contains(string(rawBefore), "SHOULD_NOT_LEAK") || contains(string(rawBefore), "vault/local/api") {
		t.Fatalf("payload leaked sensitive target or credential ref: %s", string(rawBefore))
	}

	before := decodeLocalChecksPayload(t, rawBefore)
	after := decodeLocalChecksPayload(t, rawAfter)
	results := before["results"].([]any)
	if len(results) != 4 {
		t.Fatalf("expected four check results, got %#v", results)
	}
	statuses := map[string]string{}
	for _, row := range results {
		item := row.(map[string]any)
		statuses[item["check_id"].(string)] = item["result"].(string)
	}
	if statuses["api-ok"] != "ok" || statuses["api-fail"] != "failed" || statuses["blocked-shell"] != "failed" || statuses["metrics"] != "ok" {
		t.Fatalf("unexpected lifecycle statuses %#v", statuses)
	}
	afterIntegrations := after["integrations"].([]any)
	if len(afterIntegrations) != 2 {
		t.Fatalf("expected manifest upgrade to keep v1 and add v2, got %#v", afterIntegrations)
	}
	afterResults := after["results"].([]any)
	if afterResults[0].(map[string]any)["check_id"] != "api-ok" {
		t.Fatalf("expected check config preserved after manifest upgrade, got %#v", afterResults[0])
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG66_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG66Evidence(t, evidenceDir, rawBefore, rawAfter, map[string]string{
			"checks_created":        strconv.Itoa(len(checks)),
			"checks_executed":       strconv.Itoa(len(results)),
			"ok_seen":               "yes",
			"failure_seen":          "yes",
			"blocked_kind_seen":     "yes",
			"rollback_disabled":     "yes",
			"manifest_versions":     strconv.Itoa(len(afterIntegrations)),
			"config_preserved":      "yes",
			"credential_ref_leaked": "no",
			"target_secret_leaked":  "no",
		})
	}
}

func writeManifest(t *testing.T, dir, kind, version string) {
	t.Helper()
	raw := fmt.Sprintf(`{
		"schema_version":1,
		"kind":%q,
		"version":%q,
		"status":"official",
		"owner":"aiceberg_agent",
		"permissions":["http_get"],
		"rollback":"disable local check"
	}`, kind, version)
	if err := os.WriteFile(filepath.Join(dir, kind+"-"+version+".json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func decodeLocalChecksPayload(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload map[string]map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode local checks payload: %v", err)
	}
	return payload["local_checks"]
}

func writePKG66Evidence(t *testing.T, dir string, before, after []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	files := map[string][]byte{
		"local_checks_before_upgrade.json": before,
		"local_checks_after_upgrade.json":  after,
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	keys := []string{"checks_created", "checks_executed", "ok_seen", "failure_seen", "blocked_kind_seen", "rollback_disabled", "manifest_versions", "config_preserved", "credential_ref_leaked", "target_secret_leaked"}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
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
