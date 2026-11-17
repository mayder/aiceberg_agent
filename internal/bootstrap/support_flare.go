package app

import (
	"context"
	"github.com/you/aiceberg_agent/internal/platform/support"
)

func newSupportFlare(platform string) func(context.Context) (map[string]any, error) {
	return support.NewForPlatform(platform).Collect
}
