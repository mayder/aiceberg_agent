package sysmetrics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
)

func TestCollect_RespectsPrefs(t *testing.T) {
	prefs := func() config.CollectPrefs {
		return config.CollectPrefs{
			CPU:       false,
			Memory:    false,
			Disk:      false,
			Network:   false,
			NetActive: false,
			Host:      false,
			Sensors:   false,
			Power:     false,
			Sanity:    false,
			GPU:       false,
			Services:  false,
			TimeSync:  false,
			Logs:      false,
			Updates:   false,
			Agent:     false,
			Processes: false,
			Vulns:     false,
			Inventory: false,
		}
	}

	collector := New(func() (int, int64) { return 0, 0 }, prefs)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	raw, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(raw) == 0 {
		t.Fatalf("expected payload")
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}

	caps, ok := payload["capabilities"].(map[string]any)
	if !ok || len(caps) == 0 {
		t.Fatalf("expected capabilities map")
	}
	if v, ok := caps["cpu"]; !ok || v != false {
		t.Fatalf("expected cpu capability false, got %v", caps["cpu"])
	}
	if _, ok := payload["cpu"]; ok {
		t.Fatalf("did not expect cpu section when disabled")
	}
}

func TestBuildPerformanceProfileReportsGapsWhenCollectorsDisabled(t *testing.T) {
	profile := buildPerformanceProfile(snapshot{}, config.CollectPrefs{
		CPU:       false,
		Memory:    false,
		Disk:      false,
		Network:   false,
		Sanity:    false,
		Agent:     false,
		Processes: false,
	})
	if profile == nil {
		t.Fatalf("expected performance profile with gaps")
	}
	if profile.SchemaVersion != 1 || profile.Source != "sysmetrics" {
		t.Fatalf("unexpected profile metadata: %#v", profile)
	}
	want := map[string]bool{
		"cpu_disabled":       false,
		"memory_disabled":    false,
		"disk_disabled":      false,
		"network_disabled":   false,
		"processes_disabled": false,
	}
	for _, gap := range profile.Gaps {
		if _, ok := want[gap]; ok {
			want[gap] = true
		}
	}
	for gap, found := range want {
		if !found {
			t.Fatalf("gap %q missing in %#v", gap, profile.Gaps)
		}
	}
}

func TestBuildPerformanceProfileSanitizesProcessCommand(t *testing.T) {
	mem := &memSnapshot{Total: 1000}
	profile := buildPerformanceProfile(snapshot{
		CPU:    &cpuSnapshot{PercentTotal: 91.234},
		Memory: mem,
		Processes: []procSnapshot{
			{
				PID:        123,
				Name:       "java",
				CPUPercent: 76.789,
				RSSBytes:   250,
				Cmdline:    "java -jar app.jar token=abc123 password=secret Authorization=Bearer aaa.bbb",
			},
		},
	}, config.CollectPrefs{CPU: true, Memory: true, Processes: true})

	if profile == nil || len(profile.Processes) != 1 {
		t.Fatalf("expected performance process: %#v", profile)
	}
	proc := profile.Processes[0]
	if proc.Role != "application" {
		t.Fatalf("expected application role, got %q", proc.Role)
	}
	if proc.CPUPercent != 76.79 || proc.MemPercent != 25 {
		t.Fatalf("unexpected resource percentages: %#v", proc)
	}
	if proc.Cmdline == "" || containsAny(proc.Cmdline, []string{"abc123", "secret", "aaa.bbb"}) {
		t.Fatalf("command line was not sanitized: %q", proc.Cmdline)
	}
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
