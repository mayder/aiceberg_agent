package containers

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestNormalizeContainersRedactsSensitiveLabelsAndAddsStats(t *testing.T) {
	rows := []dockerContainer{{
		ID:     "1234567890abcdef",
		Names:  []string{"/api"},
		Image:  "app:latest",
		Labels: map[string]string{"com.docker.compose.service": "api", "com.docker.compose.project": "prod", "token": "secret"},
		State:  "running",
		Status: "Up 1 minute",
	}}
	stats := dockerStats{}
	stats.CPUStats.CPUUsage.TotalUsage = 200
	stats.CPUStats.CPUUsage.PercpuUsage = []uint64{1, 2}
	stats.CPUStats.SystemUsage = 1000
	stats.PreCPUStats.CPUUsage.TotalUsage = 100
	stats.PreCPUStats.SystemUsage = 500
	stats.MemoryStats.Usage = 1024
	stats.MemoryStats.Limit = 2048
	stats.Networks = map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	}{"eth0": {RxBytes: 10, TxBytes: 20}}
	stats.BlkioStats.IoServiceBytesRecursive = []struct {
		Op    string `json:"op"`
		Value uint64 `json:"value"`
	}{{Op: "Read", Value: 30}, {Op: "Write", Value: 40}}

	inspect := dockerInspect{RestartCount: 3, LogPath: "/var/lib/docker/containers/123/123-json.log"}
	inspect.Config.User = "1000"
	out := normalizeContainers(rows, map[string]dockerStats{"1234567890ab": stats}, map[string]dockerInspect{"1234567890ab": inspect})
	if len(out) != 1 {
		t.Fatalf("expected one container, got %#v", out)
	}
	if out[0]["id"] != "1234567890ab" || out[0]["name"] != "api" {
		t.Fatalf("unexpected identity %#v", out[0])
	}
	labels := out[0]["labels"].(map[string]string)
	if labels["token"] != "[redacted]" {
		t.Fatalf("expected redacted label, got %#v", labels)
	}
	if out[0]["compose_service"] != "api" {
		t.Fatalf("expected compose service, got %#v", out[0]["compose_service"])
	}
	if out[0]["memory_usage_bytes"] != uint64(1024) {
		t.Fatalf("expected memory usage, got %#v", out[0])
	}
	if out[0]["restart_count"] != 3 || out[0]["user"] != "1000" || out[0]["namespace"] != "prod" {
		t.Fatalf("expected inspect and namespace data, got %#v", out[0])
	}
	if out[0]["network_rx_bytes"] != uint64(10) || out[0]["block_write_bytes"] != uint64(40) {
		t.Fatalf("expected network/io stats, got %#v", out[0])
	}
}

func TestFilterContainersByImageLabelNamespaceAndUser(t *testing.T) {
	rows := []dockerContainer{
		{
			ID:     "aaaaaaaaaaaa1111",
			Names:  []string{"/api"},
			Image:  "api:1",
			Labels: map[string]string{"com.docker.compose.project": "prod", "tier": "backend"},
		},
		{
			ID:     "bbbbbbbbbbbb2222",
			Names:  []string{"/worker"},
			Image:  "worker:1",
			Labels: map[string]string{"com.docker.compose.project": "dev", "tier": "jobs"},
		},
	}
	inspect := map[string]dockerInspect{
		"aaaaaaaaaaaa": {},
		"bbbbbbbbbbbb": {},
	}
	apiInspect := inspect["aaaaaaaaaaaa"]
	apiInspect.Config.User = "app"
	inspect["aaaaaaaaaaaa"] = apiInspect
	workerInspect := inspect["bbbbbbbbbbbb"]
	workerInspect.Config.User = "root"
	inspect["bbbbbbbbbbbb"] = workerInspect

	filtered := filterContainers(rows, inspect, "prod|user=app|api:1", "root|dev")
	if len(filtered) != 1 || firstName(filtered[0].Names) != "api" {
		t.Fatalf("expected only api container, got %#v", filtered)
	}
}

func TestAutodiscoveryChecksFromDockerLabels(t *testing.T) {
	rows := []dockerContainer{{
		ID:    "abcdef1234567890",
		Names: []string{"/web"},
		Image: "web:1",
		Labels: map[string]string{
			"com.docker.compose.service": "web",
			"aiceberg.ai/checks":         `[{"type":"http","url":"http://%%host%%:8080/health"}]`,
			"aiceberg.ai/check.tcp":      "8080",
		},
	}}

	checks := autodiscoveryChecks(rows)
	if len(checks) != 2 {
		t.Fatalf("expected two checks, got %#v", checks)
	}
	if checks[0]["container_id"] != "abcdef123456" || checks[0]["container_name"] != "web" {
		t.Fatalf("expected container identity on JSON check, got %#v", checks[0])
	}
	if checks[0]["service"] != "web" || checks[0]["image"] != "web:1" {
		t.Fatalf("expected service and image on check, got %#v", checks[0])
	}
	if checks[0]["kind"] != "http" || checks[0]["target"] != "http://web:8080/health" || checks[0]["enabled"] != true {
		t.Fatalf("expected canonical executable http check, got %#v", checks[0])
	}
	if checks[1]["key"] != "tcp" || checks[1]["value"] != "8080" {
		t.Fatalf("expected simple label check, got %#v", checks[1])
	}
	if checks[1]["kind"] != "tcp" || checks[1]["target"] != "web:8080" || checks[1]["enabled"] != true {
		t.Fatalf("expected canonical executable tcp check, got %#v", checks[1])
	}
}

func TestParseContainerdInfoMapsIdentityAndLabels(t *testing.T) {
	raw := []byte(`{
		"ID":"sha256:abcdef1234567890",
		"Image":"registry/app:1",
		"Labels":{
			"io.kubernetes.container.name":"api",
			"io.kubernetes.pod.namespace":"prod",
			"aiceberg.ai/check.tcp":"8080",
			"token":"secret"
		}
	}`)

	row, ok := parseContainerdInfo(raw)
	if !ok {
		t.Fatalf("expected containerd info parsed")
	}
	if shortID(row.ID) != "abcdef123456" || firstName(row.Names) != "api" {
		t.Fatalf("expected normalized identity, got %#v", row)
	}
	normalized := normalizeContainers([]dockerContainer{row}, nil, nil)
	if normalized[0]["namespace"] != "prod" {
		t.Fatalf("expected Kubernetes namespace from labels, got %#v", normalized[0])
	}
	labels := normalized[0]["labels"].(map[string]string)
	if labels["token"] != "[redacted]" {
		t.Fatalf("expected sensitive label redacted, got %#v", labels)
	}
	checks := autodiscoveryChecks([]dockerContainer{row})
	if len(checks) != 1 || checks[0]["container_id"] != "abcdef123456" {
		t.Fatalf("expected autodiscovery from containerd labels, got %#v", checks)
	}
}

func TestEffectiveRuntimeFallsBackToAutoAndAcceptsPrefs(t *testing.T) {
	c := &collector{runtime: "invalid"}
	if got := c.effectiveRuntime(); got != "auto" {
		t.Fatalf("expected auto runtime, got %q", got)
	}
	c.prefs = func() config.CollectPrefs {
		return config.CollectPrefs{
			ContainerRuntime:          "containerd",
			ContainerDockerSocket:     "/tmp/docker.sock",
			ContainerContainerdSocket: "/tmp/containerd.sock",
			ContainerContainerdNS:     "prod",
			ContainerCtrPath:          "/usr/bin/ctr",
		}
	}
	if got := c.effectiveRuntime(); got != "containerd" {
		t.Fatalf("expected prefs runtime, got %q", got)
	}
	_, dockerSocket, containerdSocket, namespace, ctrPath := c.effectiveRuntimeConfig()
	if dockerSocket != "/tmp/docker.sock" || containerdSocket != "/tmp/containerd.sock" || namespace != "prod" || ctrPath != "/usr/bin/ctr" {
		t.Fatalf("expected prefs runtime config, got docker=%q containerd=%q ns=%q ctr=%q", dockerSocket, containerdSocket, namespace, ctrPath)
	}
}

func TestReadContainerLogsWithCursorAndRedaction(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "container-json.log")
	content := strings.Join([]string{
		`{"log":"started password=secret\n","stream":"stdout","time":"2026-06-18T10:00:00Z"}`,
		`{"log":"Authorization: Bearer token-value\n","stream":"stderr","time":"2026-06-18T10:00:01Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	row := dockerContainer{
		ID:    "abcdef1234567890",
		Names: []string{"/api"},
		Image: "api:1",
		Labels: map[string]string{
			"com.docker.compose.service":  "api",
			"com.docker.compose.project":  "prod",
			"aiceberg.ai/tool-origin":     "crowdstrike",
			"aiceberg.ai/source-category": "soc",
			"aiceberg.ai/soc-source-type": "edr",
			"aiceberg.ai/soc-eligible":    "yes",
			"aiceberg.ai/route-reason":    "container_security_label",
		},
	}
	inspect := dockerInspect{LogPath: logPath}
	inspect.Config.User = "1000"
	cursor := map[string]int64{}

	events, dropped := readContainerLogFile(row, inspect, cursor, 10, 1024)
	if dropped != 0 || len(events) != 2 {
		t.Fatalf("expected two events without drops, got events=%#v dropped=%d", events, dropped)
	}
	if events[0]["service"] != "api" || events[0]["namespace"] != "prod" || events[0]["user"] != "1000" {
		t.Fatalf("expected container tags, got %#v", events[0])
	}
	for _, event := range events {
		msg, _ := event["message"].(string)
		if strings.Contains(msg, "secret") || strings.Contains(msg, "token-value") {
			t.Fatalf("container log leaked secret: %#v", event)
		}
		if event["redaction_status"] != "redacted" {
			t.Fatalf("expected redacted event, got %#v", event)
		}
		if event["aiceberg_tool_origin"] != "crowdstrike" || event["aiceberg_source_category"] != "soc" || event["aiceberg_soc_source_type"] != "edr" || event["aiceberg_origin_confidence"] != "configured" {
			t.Fatalf("expected container SOC contract, got %#v", event)
		}
	}

	again, dropped := readContainerLogFile(row, inspect, cursor, 10, 1024)
	if dropped != 0 || len(again) != 0 {
		t.Fatalf("expected cursor to skip already read lines, got events=%#v dropped=%d", again, dropped)
	}
}

func TestCollectContainerLogsBuildsPayload(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "container-json.log")
	if err := os.WriteFile(logPath, []byte(`{"log":"ok\n","stream":"stdout","time":"2026-06-18T10:00:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	row := dockerContainer{ID: "abcdef1234567890", Names: []string{"/api"}, Image: "api:1"}
	c := &collector{logsEnabled: true, logCursorPath: filepath.Join(tmp, "cursor.json"), logMaxLines: 10, logMaxBytes: 1024}

	payload := c.collectContainerLogs([]dockerContainer{row}, map[string]dockerInspect{"abcdef123456": {LogPath: logPath}})
	if payload == nil {
		t.Fatalf("expected log payload")
	}
	events := payload["events"].([]map[string]any)
	if len(events) != 1 || events[0]["container_name"] != "api" {
		t.Fatalf("unexpected events %#v", events)
	}
}

func TestPKG64ContainerLifecycleAutodiscoverySecretEvidence(t *testing.T) {
	rows := []dockerContainer{
		{
			ID:     "111111111111aaaa",
			Names:  []string{"/checkout-running"},
			Image:  "checkout:1",
			Labels: map[string]string{"com.docker.compose.service": "checkout", "com.docker.compose.project": "prod", "aiceberg.ai/check.http": "8080"},
			State:  "running",
			Status: "Up 2 minutes",
		},
		{
			ID:     "222222222222bbbb",
			Names:  []string{"/checkout-restarted"},
			Image:  "checkout:1",
			Labels: map[string]string{"com.docker.compose.service": "worker", "aiceberg.ai/check.tcp": "6379"},
			State:  "running",
			Status: "Restarted 3 times",
		},
		{
			ID:     "333333333333cccc",
			Names:  []string{"/checkout-stopped"},
			Image:  "checkout:1",
			Labels: map[string]string{"com.docker.compose.service": "stopped"},
			State:  "exited",
			Status: "Exited (0) 10 seconds ago",
		},
	}
	stats := map[string]dockerStats{}
	stats["111111111111"] = highLoadStats()
	stats["222222222222"] = highLoadStats()
	inspectByID := map[string]dockerInspect{
		"111111111111": inspectWithSensitiveFields(t, 0),
		"222222222222": inspectWithSensitiveFields(t, 3),
		"333333333333": inspectWithSensitiveFields(t, 0),
	}

	items := normalizeContainers(rows, stats, inspectByID)
	checks := autodiscoveryChecks(rows)
	payload := map[string]any{
		"containers": map[string]any{
			"schema_version":       schemaVersion,
			"source":               "docker_socket_controlled",
			"items":                items,
			"autodiscovery_checks": checks,
			"dropped_count":        0,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if strings.Contains(string(raw), "SHOULD_NOT_LEAK") || strings.Contains(string(raw), "/mounted-sensitive-file") {
		t.Fatalf("payload leaked sensitive env or volume: %s", string(raw))
	}
	if len(items) != 3 || len(checks) != 2 {
		t.Fatalf("expected 3 containers and 2 autodiscovery checks, got items=%#v checks=%#v", items, checks)
	}
	if items[1]["restart_count"] != 3 {
		t.Fatalf("expected restarted container restart_count=3, got %#v", items[1])
	}
	if items[2]["state"] != "exited" {
		t.Fatalf("expected stopped container retained, got %#v", items[2])
	}
	if cpu, _ := items[0]["cpu_percent"].(float64); cpu <= 0 {
		t.Fatalf("expected high-load CPU signal, got %#v", items[0])
	}
	if checks[0]["enabled"] != true || checks[0]["target"] == "" {
		t.Fatalf("expected new container check enabled with target, got %#v", checks)
	}

	if evidenceDir := strings.TrimSpace(os.Getenv("PKG64_EVIDENCE_DIR")); evidenceDir != "" {
		writePKG64Evidence(t, evidenceDir, raw, map[string]string{
			"containers_seen":            strconv.Itoa(len(items)),
			"running_seen":               "2",
			"stopped_seen":               "1",
			"restarted_seen":             "1",
			"high_load_seen":             "yes",
			"autodiscovery_checks":       strconv.Itoa(len(checks)),
			"new_container_check":        "yes",
			"sensitive_env_collected":    "no",
			"sensitive_volume_collected": "no",
			"redaction_or_omission":      "yes",
			"docker_real_reference":      "docs/evidence/pkg69/docker-runtime-20260619T031843Z/evidence.md",
		})
	}
}

func highLoadStats() dockerStats {
	stats := dockerStats{}
	stats.CPUStats.CPUUsage.TotalUsage = 900
	stats.CPUStats.CPUUsage.PercpuUsage = []uint64{1, 2}
	stats.CPUStats.SystemUsage = 1000
	stats.PreCPUStats.CPUUsage.TotalUsage = 100
	stats.PreCPUStats.SystemUsage = 100
	stats.MemoryStats.Usage = 256 * 1024 * 1024
	stats.MemoryStats.Limit = 512 * 1024 * 1024
	stats.Networks = map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	}{"eth0": {RxBytes: 1024, TxBytes: 2048}}
	return stats
}

func inspectWithSensitiveFields(t *testing.T, restartCount int) dockerInspect {
	t.Helper()
	raw := fmt.Sprintf(`{
		"RestartCount": %d,
		"LogPath": "/var/lib/docker/containers/controlled/controlled-json.log",
		"Config": {
			"User": "1000",
			"Env": ["SENSITIVE_ENV=SHOULD_NOT_LEAK", "SENSITIVE_KEY=SHOULD_NOT_LEAK"]
		},
		"Mounts": [{"Type": "bind", "Source": "/mounted-sensitive-file", "Destination": "/mounted-sensitive-file"}]
	}`, restartCount)
	var inspect dockerInspect
	if err := json.Unmarshal([]byte(raw), &inspect); err != nil {
		t.Fatalf("unmarshal inspect: %v", err)
	}
	return inspect
}

func writePKG64Evidence(t *testing.T, dir string, payload []byte, summary map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir evidence dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "containers_payload.json"), payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	keys := []string{"containers_seen", "running_seen", "stopped_seen", "restarted_seen", "high_load_seen", "autodiscovery_checks", "new_container_check", "sensitive_env_collected", "sensitive_volume_collected", "redaction_or_omission", "docker_real_reference"}
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s\t%s\n", key, summary[key])
	}
	if err := os.WriteFile(filepath.Join(dir, "SUMMARY.tsv"), []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}
