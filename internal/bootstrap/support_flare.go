package app

import (
	"context"
	"strings"

	"github.com/you/aiceberg_agent/internal/platform/support"
)

type supportLogReader interface{ SupportLog() (string, bool) }

func newSupportFlare(platform string, logs ...supportLogReader) func(context.Context) (map[string]any, error) {
	collect := support.NewForPlatform(platform).Collect
	return func(ctx context.Context) (map[string]any, error) {
		result, err := collect(ctx)
		if err == nil || ctx.Err() != nil || (platform != "linux" && platform != "windows") || len(logs) == 0 || logs[0] == nil {
			return result, err
		}
		text, truncated := logs[0].SupportLog()
		if strings.TrimSpace(text) == "" {
			return result, err
		}
		rows, _ := result["sources"].([]any)
		rows = append(rows, map[string]any{"source": "agent_service_process_log", "status": "collected", "text": text, "truncated": truncated, "volatile": true, "max_age_seconds": 3600})
		result["sources"] = rows
		result["status"] = "collected"
		result["degraded"] = true
		return result, nil
	}
}
