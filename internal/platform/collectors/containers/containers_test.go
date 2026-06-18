package containers

import "testing"

func TestNormalizeContainersRedactsSensitiveLabelsAndAddsStats(t *testing.T) {
	rows := []dockerContainer{{
		ID:     "1234567890abcdef",
		Names:  []string{"/api"},
		Image:  "app:latest",
		Labels: map[string]string{"com.docker.compose.service": "api", "token": "secret"},
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

	out := normalizeContainers(rows, map[string]dockerStats{"1234567890ab": stats})
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
	if out[0]["network_rx_bytes"] != uint64(10) || out[0]["block_write_bytes"] != uint64(40) {
		t.Fatalf("expected network/io stats, got %#v", out[0])
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
