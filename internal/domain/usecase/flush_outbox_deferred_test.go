package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

// This fixture removes only acknowledged IDs, so a second turn proves delivery
// of the remaining envelopes rather than counting the same successful IDs twice.
type deferredOutbox struct {
	mu      sync.Mutex
	pending []entities.Envelope
	acked   map[string]int
}

type deferredTransportFunc func([]entities.Envelope, string, string) ([]byte, error)

func (f deferredTransportFunc) SendWithAuth(batch []entities.Envelope, auth, endpoint string) ([]byte, error) {
	return f(batch, auth, endpoint)
}

func TestFlushOutboxCatchupPreservesInvalidCredentialCooldown(t *testing.T) {
	repo := &deferredOutbox{acked: make(map[string]int)}
	for _, auth := range []string{"Token fixture-good", "Token fixture-bad"} {
		_ = repo.Append(entities.Envelope{ID: auth, AuthHeader: auth, Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)})
	}
	var goodCalls, badCalls atomic.Int32
	tx := deferredTransportFunc(func(_ []entities.Envelope, auth, _ string) ([]byte, error) {
		if auth == "Token fixture-bad" {
			badCalls.Add(1)
			return nil, fakeHTTPStatusErr{code: 401}
		}
		goodCalls.Add(1)
		return []byte(`{"received":1,"skipped":0}`), nil
	})
	uc := NewFlushOutboxWithOptions(repo, tx, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchSize: 2, Concurrency: 2})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	uc.now = func() time.Time { return now }
	if n, err := uc.Execute(context.Background()); n != 1 || err == nil || uc.Snapshot().LastDeferred != 0 {
		t.Fatal("partial auth failure not isolated")
	}
	if n, err := uc.Execute(context.Background()); n != 0 || err == nil || badCalls.Load() != 1 {
		t.Fatal("early next turn bypassed credential cooldown")
	}
	now = now.Add(61 * time.Second)
	if n, err := uc.Execute(context.Background()); n != 0 || err == nil || badCalls.Load() != 2 {
		t.Fatal("credential did not retry after the existing cooldown")
	}
	if pending, _ := repo.Len(); pending != 1 || goodCalls.Load() != 1 || repo.acked["Token fixture-bad"] != 0 {
		t.Fatal("failed identity removed or successful identity replayed")
	}
}

func (r *deferredOutbox) Append(e entities.Envelope) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, e)
	return nil
}

func (r *deferredOutbox) ReadBatch(n int) ([]entities.Envelope, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.pending) {
		n = len(r.pending)
	}
	return append([]entities.Envelope(nil), r.pending[:n]...), nil
}

func (r *deferredOutbox) Ack(ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		r.acked[id]++
	}
	remaining := make([]entities.Envelope, 0, len(r.pending))
	for _, e := range r.pending {
		if r.acked[e.ID] == 0 {
			remaining = append(remaining, e)
		}
	}
	r.pending = remaining
	return nil
}

func (r *deferredOutbox) Len() (int, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending), 0
}

func TestFlushOutboxDeferredGroupsResumeWithoutDuplicateAck(t *testing.T) {
	repo := &deferredOutbox{acked: make(map[string]int)}
	for i := 0; i < 16; i++ {
		_ = repo.Append(entities.Envelope{ID: fmt.Sprint(i), AuthHeader: fmt.Sprintf("Token fixture-%d", i),
			Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)})
	}
	tx := &parallelFlushTransport{started: make(chan string, 16), release: make(chan struct{})}
	uc := NewFlushOutboxWithOptions(repo, tx, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchSize: 16, Concurrency: 2})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	var n int
	var err error
	go func() { n, err = uc.Execute(ctx); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-tx.started:
		case <-time.After(2 * time.Second):
			t.Fatal("two independent groups did not start")
		}
	}
	// Cancel the dispatch context while the two in-flight requests finish safely.
	cancel()
	close(tx.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch did not finish")
	}
	if n != 2 || !errors.Is(err, context.Canceled) || uc.Snapshot().LastDeferred != 14 || uc.Snapshot().LastRetained != 14 || uc.Snapshot().LastReadBatch != 16 {
		t.Fatalf("acked=%d err=%v snapshot=%+v", n, err, uc.Snapshot())
	}
	if pending, _ := repo.Len(); pending != 14 {
		t.Fatalf("unsent work lost: pending=%d", pending)
	}
	n, err = uc.Execute(context.Background())
	if err != nil || n != 14 || uc.Snapshot().LastDeferred != 0 || uc.Snapshot().LastRetained != 0 {
		t.Fatalf("resume acked=%d err=%v snapshot=%+v", n, err, uc.Snapshot())
	}
	if pending, _ := repo.Len(); pending != 0 || tx.max.Load() > 2 || len(repo.acked) != 16 {
		t.Fatalf("pending=%d max_concurrency=%d distinct_acks=%d", pending, tx.max.Load(), len(repo.acked))
	}
	for id, count := range repo.acked {
		if count != 1 {
			t.Fatalf("id=%s acknowledged %d times", id, count)
		}
	}
}

func TestFlushOutboxRefusedAckIsRetainedButNotDispatchDeferred(t *testing.T) {
	repo := &fakeOutbox{batch: []entities.Envelope{{ID: "one", Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)}}}
	tx := &fakeTransport{body: []byte(`{"received":0,"skipped":1,"errors_by_reason":{"invalid_token":1}}`)}
	uc := NewFlushOutbox(repo, tx, &fakeLogger{}, "", nil)
	if n, err := uc.Execute(context.Background()); n != 0 || err == nil || len(repo.acked) != 0 {
		t.Fatal("unsafe response acknowledged data")
	}
	if s := uc.Snapshot(); s.LastRetained != 1 || s.LastDeferred != 0 {
		t.Fatalf("refusal confused with undispatched work: %+v", s)
	}
}
