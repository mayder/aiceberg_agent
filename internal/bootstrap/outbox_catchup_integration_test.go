package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/local/outbox"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/data/repositories"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/usecase"
)

type catchupAPI struct {
	mu         sync.Mutex
	received   map[string]int
	badCalls   atomic.Int32
	active     atomic.Int32
	maxActive  atomic.Int32
	violations atomic.Int32
}

func (a *catchupAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := a.active.Add(1)
	defer a.active.Add(-1)
	for old := a.maxActive.Load(); n > old; old = a.maxActive.Load() {
		if a.maxActive.CompareAndSwap(old, n) {
			break
		}
	}
	if r.Header.Get("Authorization") == "Token fixture-invalid" {
		a.badCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var batch []entities.Envelope
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil || len(batch) == 0 {
		a.violations.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	for _, e := range batch {
		if r.Header.Get("Authorization") != "Token "+e.AgentID || e.Endpoint != r.URL.Path {
			a.violations.Add(1)
		}
		a.received[e.ID]++
	}
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"received": len(batch), "skipped": 0})
}

func TestOutboxCatchupDrainsPersistentBacklogWithRealHTTPAndRetains401(t *testing.T) {
	api := &catchupAPI{received: make(map[string]int)}
	server := httptest.NewServer(api)
	defer server.Close()
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := outbox.NewBoltStore(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seedCatchupBacklog(t, store)
	cfg := config.Config{APIBaseURL: server.URL, IngestTimeout: time.Second,
		OutboxFlushInterval: 15 * time.Second, HubOutboxCatchupEnabled: true}
	uc := usecase.NewFlushOutboxWithOptions(repositories.NewOutboxRepository(store),
		transport.NewHTTPJSONClient(cfg), testBootstrapLogger{}, "", nil,
		hubOutboxFlushOptions("hub", cfg, usecase.FlushOutboxOptions{}))
	n, err := uc.Execute(context.Background())
	if n != 799 || err == nil || outboxCatchupDelay("hub", cfg, n, uc.Snapshot(), nil) != time.Second {
		t.Fatalf("first turn acked=%d err=%v snapshot=%+v", n, err, uc.Snapshot())
	}
	n, err = uc.Execute(context.Background())
	if n != 101 || err == nil || outboxCatchupDelay("hub", cfg, n, uc.Snapshot(), nil) != 0 {
		t.Fatalf("second turn acked=%d err=%v snapshot=%+v", n, err, uc.Snapshot())
	}
	if n, err = uc.Execute(context.Background()); n != 0 || err == nil || api.badCalls.Load() != 1 {
		t.Fatal("no-progress turn bypassed 401 cooldown")
	}
	if count, _ := store.Len(); count != 1 || api.violations.Load() != 0 || api.maxActive.Load() > 8 {
		t.Fatalf("pending=%d violations=%d max_concurrency=%d", count, api.violations.Load(), api.maxActive.Load())
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.received) != 900 {
		t.Fatalf("delivered distinct frames=%d want=900", len(api.received))
	}
	for id, count := range api.received {
		if count != 1 {
			t.Fatalf("frame=%s duplicate delivery=%d", id, count)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := outbox.NewBoltStore(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pending, err := reopened.Peek(2)
	if err != nil || len(pending) != 1 || pending[0].ID != "0000-invalid" {
		t.Fatal("unacknowledged envelope did not survive reopen")
	}
}

func seedCatchupBacklog(t *testing.T, store *outbox.BoltStore) {
	t.Helper()
	if err := store.Push(entities.Envelope{ID: "0000-invalid", AuthHeader: "Token fixture-invalid",
		Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 900; i++ {
		agent := fmt.Sprintf("fixture-relay-%03d", i%615)
		route := "/v1/ingest/metrics"
		if i%10 == 0 {
			route = "/v1/ingest/health"
		}
		if err := store.Push(entities.Envelope{ID: fmt.Sprintf("%04d-frame", i), AgentID: agent,
			AuthHeader: "Token " + agent, Endpoint: route, Body: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
}
