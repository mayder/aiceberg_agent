package sysmetrics

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestBoundedContextUsesShorterCollectorDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), time.Minute)
	defer cancelParent()

	ctx, cancel := boundedContext(parent, 20*time.Millisecond)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected bounded deadline")
	}
	if remaining := time.Until(deadline); remaining > time.Second {
		t.Fatalf("expected short bounded deadline, remaining=%s", remaining)
	}
}

func TestBoundedContextKeepsShorterParentDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelParent()

	ctx, cancel := boundedContext(parent, time.Minute)
	defer cancel()

	parentDeadline, _ := parent.Deadline()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatalf("expected parent deadline")
	}
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("expected parent deadline preserved, got %s want %s", deadline, parentDeadline)
	}
}

func TestLinuxSecurityUpdatesSkipsDnfByDefault(t *testing.T) {
	t.Setenv("AICEBERG_AGENT_ENABLE_DNF_UPDATEINFO", "")

	got := linuxSecurityUpdates(time.Second)
	if got.Source != "dnf" {
		t.Fatalf("unexpected source: %#v", got)
	}
	if !strings.Contains(got.Error, "skipped") {
		t.Fatalf("expected dnf skipped by default, got %#v", got)
	}
}

func TestListLinuxPackagesSkipsFullInventoryByDefault(t *testing.T) {
	t.Setenv("AICEBERG_AGENT_ENABLE_PACKAGE_INVENTORY", "")

	if got := listLinuxPackages(context.Background()); len(got) != 0 {
		t.Fatalf("expected package inventory skipped by default, got %d packages", len(got))
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
	if profile.AgentRuntime == nil || profile.AgentRuntime.Version == "" {
		t.Fatalf("expected agent runtime telemetry: %#v", profile.AgentRuntime)
	}
}

func TestAgentRuntimeProfileReportsOwnFootprint(t *testing.T) {
	dir := t.TempDir()
	outbox := filepath.Join(dir, "outbox.db")
	if err := os.WriteFile(outbox, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write outbox: %v", err)
	}
	t.Setenv("OUTBOX_PATH", outbox)
	t.Setenv("OUTBOX_MAX_MB", "1")
	t.Setenv("AGENT_MODE", "relay")

	profile := buildAgentRuntimeProfile(snapshot{Memory: &memSnapshot{Total: 1024}}, config.CollectPrefs{Memory: true})

	if profile == nil {
		t.Fatalf("expected runtime profile")
	}
	if profile.PID <= 0 || profile.Version == "" || profile.Mode != "relay" {
		t.Fatalf("unexpected runtime identity: %#v", profile)
	}
	foundOutbox := false
	for _, location := range profile.StorageLocations {
		if location.Kind == "outbox" {
			foundOutbox = true
			if !location.Exists || location.SizeBytes != 7 || location.Status != "ok" {
				t.Fatalf("unexpected outbox footprint: %#v", location)
			}
		}
		if strings.Contains(location.Path, os.Getenv("HOME")) {
			t.Fatalf("path was not sanitized: %#v", location)
		}
	}
	if !foundOutbox {
		t.Fatalf("expected outbox footprint in %#v", profile.StorageLocations)
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
