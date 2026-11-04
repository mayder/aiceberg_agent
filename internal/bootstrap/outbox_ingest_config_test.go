package app

import (
	"context"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestHubWorkerQueuesOwnConfigAndPreservesRelayRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	applied := make(chan string, 3)
	apply := func(auth string, _ usecase.IngestConfig) { applied <- auth }
	handle, events := queueOwnIngestConfig(ctx, "Token owner", apply)
	handle("Token owner", usecase.IngestConfig{})
	if len(applied) != 0 || len(events) != 1 {
		t.Fatal("worker applied own preferences concurrently")
	}
	handle("Token relay", usecase.IngestConfig{})
	if got := <-applied; got != "Token relay" {
		t.Fatal("relay configuration lost its auth boundary")
	}
	event := <-events
	apply(event.auth, event.config)
	if got := <-applied; got != "Token owner" {
		t.Fatal("main-loop delivery changed the owner")
	}
	handle("Token owner", usecase.IngestConfig{})
	done := make(chan struct{})
	go func() { handle("Token owner", usecase.IngestConfig{}); close(done) }()
	cancel()
	awaitOutboxSignal(t, done, "config queue cancellation")
}
