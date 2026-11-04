package app

import (
	"context"

	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type ingestConfigEvent struct {
	auth   string
	config usecase.IngestConfig
}

func confirmOwnIngestConfig(owner string, currentVersion func() string, requestFullSync func(), apply usecase.IngestConfigHandler) usecase.IngestConfigHandler {
	return func(auth string, cfg usecase.IngestConfig) {
		if auth != owner {
			apply(auth, cfg)
			return
		}
		before := currentVersion()
		apply(auth, cfg)
		if after := currentVersion(); after != "" && after != before {
			requestFullSync()
		}
	}
}

// Own configuration remains on the main loop beside config-sync. Relay config
// can use the existing concurrent PendingConfigStore without touching own prefs.
func queueOwnIngestConfig(ctx context.Context, owner string, apply usecase.IngestConfigHandler) (usecase.IngestConfigHandler, <-chan ingestConfigEvent) {
	events := make(chan ingestConfigEvent, 1)
	return func(auth string, cfg usecase.IngestConfig) {
		if auth != owner {
			apply(auth, cfg)
			return
		}
		select {
		case events <- ingestConfigEvent{auth, cfg}:
		case <-ctx.Done():
		}
	}, events
}
