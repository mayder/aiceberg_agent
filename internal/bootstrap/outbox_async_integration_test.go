package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestAsyncHubAcceptsRelayDuringBlockedAPIAndPreservesPendingOnShutdown(t *testing.T) {
	apiStarted, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	releaseAPI := func() { releaseOnce.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch []entities.Envelope
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil || len(batch) != 1 ||
			r.Header.Get("Authorization") != "Token fixture-relay" ||
			r.Header.Get("X-Agent-Identity") != "fixture-identity" || r.URL.Path != "/v1/ingest/metrics" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		apiStarted <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"received":1,"skipped":0}`))
	}))
	t.Cleanup(func() { releaseAPI(); upstream.Close() })
	fixture := newAsyncOutboxFixture(t, upstream.URL, releaseAPI)
	store, repo := fixture.store, fixture.repo
	results, done, cancel := fixture.results, fixture.done, fixture.cancel

	relayStore := outbox.NewMemStore()
	relayRepo := repositories.NewOutboxRepository(relayStore)
	if err := relayRepo.Append(entities.Envelope{ID: "fixture-frame", AgentID: "fixture-relay", Kind: "metric",
		AuthHeader: "Token fixture-relay", IdentityHeader: "fixture-identity", Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	relay := usecase.NewFlushOutbox(relayRepo, transport.NewHubClient(config.Config{HubURL: fixture.hubURL}), testBootstrapLogger{}, "", nil)
	relayDone := make(chan struct{})
	go func() {
		defer close(relayDone)
		if n, err := relay.Execute(context.Background()); n != 1 || err != nil {
			t.Errorf("relay did not receive durable hub acceptance: ack=%d err=%v", n, err)
		}
	}()
	awaitOutboxSignal(t, relayDone, "relay acceptance independent of API")
	awaitOutboxSignal(t, apiStarted, "arrival-triggered upstream request")
	if pending, _ := store.Len(); pending != 1 {
		t.Fatalf("hub discarded before upstream ACK: pending=%d", pending)
	}
	if pending, _ := relayStore.Len(); pending != 0 {
		t.Fatal("accepted relay data remained pending at the source")
	}
	for _, id := range []string{"pending-one", "pending-two"} {
		if err := repo.Append(entities.Envelope{ID: id, AgentID: "fixture-relay",
			AuthHeader: "Token fixture-relay", Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	select {
	case <-done:
		t.Fatal("worker shutdown did not wait for the in-flight request")
	case <-time.After(20 * time.Millisecond):
	}
	releaseAPI()
	if result := awaitOutboxCycle(t, results); result.acked != 1 || result.failed {
		t.Fatalf("upstream completion was not acknowledged: %+v", result)
	}
	awaitOutboxSignal(t, done, "worker joined after API completion")
	if pending, _ := store.Len(); pending != 2 {
		t.Fatal("shutdown sent or discarded pending frames")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := outbox.NewBoltStore(fixture.path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	frames, err := reopened.Peek(3)
	if err != nil || len(frames) != 2 || frames[0].ID != "pending-one" || frames[1].ID != "pending-two" {
		t.Fatal("pending frames did not survive store reopen")
	}
}

func awaitOutboxCycle(t *testing.T, results <-chan outboxCycleResult) outboxCycleResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("outbox cycle did not complete")
		return outboxCycleResult{}
	}
}
