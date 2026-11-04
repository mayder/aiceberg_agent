package app

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/ports"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
	"github.com/you/aiceberg_agent/internal/interfaces/hub"
)

type asyncOutboxFixture struct {
	store   *outbox.BoltStore
	repo    ports.OutboxRepo
	path    string
	hubURL  string
	results chan outboxCycleResult
	done    chan struct{}
	cancel  context.CancelFunc
	prunes  atomic.Int32
}

func newAsyncOutboxFixture(t *testing.T, apiURL string, releaseAPI func()) *asyncOutboxFixture {
	t.Helper()
	f := &asyncOutboxFixture{path: filepath.Join(t.TempDir(), "hub.db"), results: make(chan outboxCycleResult, 8), done: make(chan struct{})}
	var err error
	f.store, err = outbox.NewBoltStoreWithOptions(f.path, 4, outbox.BoltOptions{BatchWrites: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.store.Close() })
	wake := make(chan struct{}, 1)
	f.repo = repositories.NewOutboxRepositoryWithNotify(f.store, func() { signalOutbox(wake) })
	server := httptest.NewServer(hub.NewHandler(config.Config{}, f.repo, testBootstrapLogger{}, nil))
	f.hubURL = server.URL
	t.Cleanup(server.Close)
	cfg := config.Config{APIBaseURL: apiURL, IngestTimeout: 15 * time.Second,
		OutboxFlushInterval: time.Hour, HubOutboxAsyncEnabled: true, HubOutboxCatchupEnabled: true}
	flush := usecase.NewFlushOutboxWithOptions(f.repo, transport.NewHTTPJSONClient(cfg), testBootstrapLogger{}, "", nil,
		hubOutboxFlushOptions("hub", cfg, usecase.FlushOutboxOptions{}))
	cycle := &outboxFlushCycle{mode: "hub", cfg: cfg, main: flush, counter: &obsCounters{},
		prune: func() { f.prunes.Add(1) }, report: func(context.Context, string, string, string, error, map[string]any) {}}
	var ctx context.Context
	ctx, f.cancel = context.WithCancel(context.Background())
	go func() {
		defer close(f.done)
		runHubOutboxWorker(ctx, wake, time.Hour, func(ctx context.Context) outboxCycleResult {
			r := cycle.Execute(ctx)
			f.results <- r
			return r
		})
	}()
	t.Cleanup(func() { f.cancel(); releaseAPI(); awaitOutboxSignal(t, f.done, "async worker stop") })
	awaitOutboxCycle(t, f.results)
	return f
}
