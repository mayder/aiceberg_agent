package sysmetrics

import (
	"context"
	"encoding/json"
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
