package containers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if checks[1]["key"] != "tcp" || checks[1]["value"] != "8080" {
		t.Fatalf("expected simple label check, got %#v", checks[1])
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
		ID:     "abcdef1234567890",
		Names:  []string{"/api"},
		Image:  "api:1",
		Labels: map[string]string{"com.docker.compose.service": "api", "com.docker.compose.project": "prod"},
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
