package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type parallelFlushRepo struct {
	batch []entities.Envelope
	mu    sync.Mutex
	acked []string
}

func (r *parallelFlushRepo) Append(e entities.Envelope) error {
	r.batch = append(r.batch, e)
	return nil
}
func (r *parallelFlushRepo) ReadBatch(n int) ([]entities.Envelope, error) {
	if n > len(r.batch) {
		n = len(r.batch)
	}
	return r.batch[:n], nil
}
func (r *parallelFlushRepo) Ack(ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.acked = append(r.acked, ids...)
	return nil
}
func (r *parallelFlushRepo) Len() (int, int64) { return len(r.batch), 0 }

type parallelFlushTransport struct {
	started     chan string
	release     chan struct{}
	active, max atomic.Int32
}

func (tx *parallelFlushTransport) SendWithAuth(batch []entities.Envelope, auth, endpoint string) ([]byte, error) {
	n := tx.active.Add(1)
	defer tx.active.Add(-1)
	for old := tx.max.Load(); n > old; old = tx.max.Load() {
		if tx.max.CompareAndSwap(old, n) {
			break
		}
	}
	tx.started <- auth
	<-tx.release
	if auth == "Token bad" {
		return nil, errors.New("destination unavailable")
	}
	return []byte(`{"received":1,"skipped":0}`), nil
}

func TestFlushOutboxParallelBoundPreservesFailedIdentityAndSuccessfulAcks(t *testing.T) {
	repo := &parallelFlushRepo{}
	for _, auth := range []string{"Token a", "Token b", "Token c", "Token bad"} {
		repo.batch = append(repo.batch, entities.Envelope{ID: auth, AuthHeader: auth, Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)})
	}
	tx := &parallelFlushTransport{started: make(chan string, 4), release: make(chan struct{})}
	uc := NewFlushOutboxWithOptions(repo, tx, &fakeLogger{}, "", nil, FlushOutboxOptions{BatchSize: 4, Concurrency: 2})
	done := make(chan struct{})
	var acked int
	var err error
	go func() { acked, err = uc.Execute(context.Background()); close(done) }()
	for i := 0; i < 2; i++ {
		select {
		case <-tx.started:
		case <-time.After(2 * time.Second):
			t.Fatal("independent groups were serialized")
		}
	}
	if tx.active.Load() != 2 {
		t.Fatal("wrong concurrency")
	}
	close(tx.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flush did not finish")
	}
	if tx.max.Load() != 2 || acked != 3 || len(repo.acked) != 3 || err == nil {
		t.Fatalf("max=%d acked=%d ids=%v err=%v", tx.max.Load(), acked, repo.acked, err)
	}
	for _, id := range repo.acked {
		if id == "Token bad" {
			t.Fatal("failed identity was acknowledged")
		}
	}
	if uc.Snapshot().LastRetained != 1 {
		t.Fatal("failure was not retained")
	}
}

func TestFlushOutboxParallelCancelledDispatchRetainsAllUnsentGroups(t *testing.T) {
	repo := &parallelFlushRepo{batch: []entities.Envelope{{ID: "one", AuthHeader: "Token a", Body: json.RawMessage(`{}`)}, {ID: "two", AuthHeader: "Token b", Body: json.RawMessage(`{}`)}}}
	tx := &parallelFlushTransport{started: make(chan string, 2), release: make(chan struct{})}
	uc := NewFlushOutboxWithOptions(repo, tx, &fakeLogger{}, "", nil, FlushOutboxOptions{Concurrency: 2})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := uc.Execute(ctx)
	if n != 0 || !errors.Is(err, context.Canceled) || len(repo.acked) != 0 || len(tx.started) != 0 || uc.Snapshot().LastRetained != 2 {
		t.Fatal("cancelled dispatch sent or acknowledged data")
	}
}
