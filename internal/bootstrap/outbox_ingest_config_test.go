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

func TestIngestConfirmationOnlyRequestsSyncForChangedOwnerVersion(t *testing.T) {
	for _, tc := range []struct {
		name, auth, before, after string
		want                      int
	}{
		{"changed owner", "owner", "40", "41", 1},
		{"duplicate owner", "owner", "41", "41", 0},
		{"relay boundary", "relay", "40", "41", 0},
		{"failed persistence", "owner", "40", "40", 0},
		{"invalid empty version", "owner", "40", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version, requests, applied := tc.before, 0, 0
			handle := confirmOwnIngestConfig("owner", func() string { return version }, func() { requests++ }, func(auth string, _ usecase.IngestConfig) {
				if auth != tc.auth {
					t.Fatal("authentication boundary changed")
				}
				applied++
				version = tc.after
			})
			handle(tc.auth, usecase.IngestConfig{})
			if requests != tc.want || applied != 1 {
				t.Fatalf("requests=%d applied=%d", requests, applied)
			}
		})
	}
}
