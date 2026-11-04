package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

func TestAsyncRelaySendsDuringBlockedCollectorThroughRealHub(t *testing.T) {
	received := make(chan []entities.Envelope, 8)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token synthetic-relay" || r.Header.Get("X-Agent-Identity") != "relay-identity" || r.URL.Path != "/v1/ingest/metrics" {
			t.Error("relay auth/route changed")
		}
		var batch []entities.Envelope
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		received <- batch
		_, _ = w.Write([]byte(`{"received":1,"skipped":0}`))
	}))
	defer api.Close()
	h := newAsyncOutboxFixture(t, api.URL, func() {})
	store, err := outbox.NewBoltStore(filepath.Join(t.TempDir(), "relay.db"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg := config.Config{AgentMode: "relay", HubURL: h.hubURL, RelayOutboxAsyncEnabled: true, OutboxFlushInterval: time.Hour}
	if !asyncOutboxEnabled("relay", cfg) {
		t.Fatal("relay worker disabled")
	}
	wake := make(chan struct{}, 1)
	repo := repositories.NewOutboxRepositoryWithNotify(store, func() { signalOutbox(wake) })
	flush := usecase.NewFlushOutbox(repo, transport.NewHubClient(cfg), testBootstrapLogger{}, "Token synthetic-relay", nil)
	cycle := &outboxFlushCycle{mode: "relay", cfg: cfg, main: flush, counter: &obsCounters{}, report: func(context.Context, string, string, string, error, map[string]any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runAsyncOutboxWorker(ctx, wake, time.Hour, cycle.Execute) }()
	defer func() { cancel(); awaitOutboxSignal(t, done, "relay worker stop") }()
	collectorBlocked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer func() { once.Do(func() { close(release) }) }()
	go func() { close(collectorBlocked); <-release }()
	awaitOutboxSignal(t, collectorBlocked, "another collector blocked")
	env := entities.Envelope{ID: "ready-relay", AgentID: "synthetic-relay", TSUnixMs: 42, Endpoint: "/v1/ingest/metrics", AuthHeader: "Token synthetic-relay", IdentityHeader: "relay-identity", Body: json.RawMessage(`{"value":42}`)}
	if err := repo.Append(env); err != nil {
		t.Fatal(err)
	}
	select {
	case batch := <-received:
		if len(batch) != 1 || batch[0].ID != env.ID || batch[0].Meta["hub_received_at_unix_ms"] == "" {
			t.Fatal("forwarded receipt changed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ready frame waited for collector or hourly timer")
	}
	select {
	case <-release:
		t.Fatal("collector was released before send")
	default:
	}
}

func TestRelayCatchupRequiresProgressAndPruningKeepsCadence(t *testing.T) {
	cfg := config.Config{RelayOutboxAsyncEnabled: true, OutboxFlushInterval: time.Minute}
	snapshot := usecase.FlushOutboxSnapshot{BatchLimit: 50, LastReadBatch: 50}
	if outboxCatchupDelay("relay", cfg, 50, snapshot, nil) != time.Second {
		t.Fatal("relay backlog left waiting")
	}
	if outboxCatchupDelay("relay", cfg, 0, snapshot, nil) != 0 {
		t.Fatal("no-progress retry accelerated")
	}
	cfg.RelayOutboxAsyncEnabled = false
	if outboxCatchupDelay("relay", cfg, 50, snapshot, nil) != 0 {
		t.Fatal("opt-out ignored")
	}
	cfg.RelayOutboxAsyncEnabled = true
	prunes := 0
	cycle := outboxFlushCycle{mode: "relay", cfg: cfg, prune: func() { prunes++ }}
	cycle.startRelayPruneCadence(time.Now())
	for i := 0; i < 100; i++ {
		cycle.Execute(context.Background())
	}
	if prunes != 0 {
		t.Fatalf("arrival advanced startup retention: %d", prunes)
	}
	cycle.cadence.lastPrune = time.Now().Add(-2 * time.Minute)
	cycle.Execute(context.Background())
	if prunes != 1 {
		t.Fatal("periodic retention cadence lost")
	}
}

func TestAsyncRelayRetainsCommittedEnvelopeAfterHubFailure(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer target.Close()
	path := filepath.Join(t.TempDir(), "relay.db")
	s, err := outbox.NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	wake := make(chan struct{}, 1)
	repo := repositories.NewOutboxRepositoryWithNotify(s, func() { signalOutbox(wake) })
	cfg := config.Config{AgentMode: "relay", HubURL: target.URL, RelayOutboxAsyncEnabled: true, OutboxFlushInterval: time.Hour}
	flush := usecase.NewFlushOutbox(repo, transport.NewHubClient(cfg), testBootstrapLogger{}, "", nil)
	cycle := outboxFlushCycle{mode: "relay", cfg: cfg, main: flush, counter: &obsCounters{}, report: func(context.Context, string, string, string, error, map[string]any) {}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	results := make(chan outboxCycleResult, 4)
	go func() {
		defer close(done)
		runAsyncOutboxWorker(ctx, wake, time.Hour, func(ctx context.Context) outboxCycleResult { r := cycle.Execute(ctx); results <- r; return r })
	}()
	defer func() { cancel(); awaitOutboxSignal(t, done, "failed relay worker stop"); s.Close() }()
	awaitOutboxCycle(t, results)
	env := entities.Envelope{ID: "must-retain", AgentID: "synthetic", Endpoint: "/v1/ingest/metrics", AuthHeader: "Token synthetic", IdentityHeader: "identity", Body: json.RawMessage(`{"value":42}`)}
	if err := repo.Append(env); err != nil {
		t.Fatal(err)
	}
	result := awaitOutboxCycle(t, results)
	if !result.failed || result.acked != 0 {
		t.Fatal("HUB failure lost pending data")
	}
	cancel()
	awaitOutboxSignal(t, done, "relay worker joined")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := outbox.NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	frames, err := old.Peek(2)
	if err != nil || len(frames) != 1 || frames[0].ID != env.ID || frames[0].AuthHeader != env.AuthHeader || frames[0].IdentityHeader != env.IdentityHeader {
		t.Fatal("pending envelope changed after restart")
	}
}
