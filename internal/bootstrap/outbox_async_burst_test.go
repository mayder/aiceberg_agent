package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/common/config"
	"github.com/you/aiceberg_agent/internal/data/remote/transport"
	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAsyncHubConcurrentRelayBurstRemainsBoundedAndDrains(t *testing.T) {
	const relays = 64
	api := &catchupAPI{received: make(map[string]int)}
	release, started := make(chan struct{}), make(chan struct{}, relays)
	var once sync.Once
	releaseAPI := func() { once.Do(func() { close(release) }) }
	var active, max atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := max.Load(); n > old; old = max.Load() {
			if max.CompareAndSwap(old, n) {
				break
			}
		}
		started <- struct{}{}
		<-release
		api.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { releaseAPI(); upstream.Close() })
	f := newAsyncOutboxFixture(t, upstream.URL, releaseAPI)
	client := transport.NewHubClient(config.Config{HubURL: f.hubURL})
	errors := make(chan error, relays)
	for i := 0; i < relays; i++ {
		go func(i int) {
			agent := fmt.Sprintf("fixture-relay-%02d", i)
			_, err := client.SendWithAuth([]entities.Envelope{{ID: agent, AgentID: agent,
				Endpoint: "/v1/ingest/metrics", Body: json.RawMessage(`{}`)}}, "Token "+agent, "/v1/ingest/metrics")
			errors <- err
		}(i)
	}
	for i := 0; i < relays; i++ {
		select {
		case err := <-errors:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("receiver waited for the blocked upstream API")
		}
	}
	awaitOutboxSignal(t, started, "immediate burst forwarding")
	if pending, _ := f.store.Len(); pending != relays {
		t.Fatalf("unacknowledged relay frames lost: pending=%d", pending)
	}
	releaseAPI()
	for acked := 0; acked < relays; {
		result := awaitOutboxCycle(t, f.results)
		if result.failed {
			t.Fatal("healthy burst failed")
		}
		acked += result.acked
	}
	if pending, _ := f.store.Len(); pending != 0 || max.Load() > 8 || f.prunes.Load() != 1 || api.violations.Load() != 0 {
		t.Fatalf("pending=%d concurrency=%d prunes=%d violations=%d", pending, max.Load(), f.prunes.Load(), api.violations.Load())
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.received) != relays {
		t.Fatal("missing delivered relays")
	}
	for _, count := range api.received {
		if count != 1 {
			t.Fatal("duplicate delivery")
		}
	}
}
