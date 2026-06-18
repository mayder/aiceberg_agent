package outbox

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestBoltStoreRejectsOversizedEnvelopeWithoutPartialWrite(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("new bolt store: %v", err)
	}
	defer store.Close()

	err = store.Push(entities.Envelope{
		ID:            "too-large",
		AgentID:       "agent-1",
		Kind:          "metric",
		SchemaVersion: 1,
		TSUnixMs:      time.Now().UnixMilli(),
		Body:          map[string]any{"payload": strings.Repeat("x", 1024*1024+128)},
	})
	if err == nil {
		t.Fatalf("expected oversized envelope to be rejected")
	}
	if !strings.Contains(err.Error(), "outbox full") {
		t.Fatalf("expected outbox full error, got %v", err)
	}
	count, bytes := store.Len()
	if count != 0 || bytes != 0 {
		t.Fatalf("expected no partial write, got count=%d bytes=%d", count, bytes)
	}
}

func TestBoltStorePersistsReplayUntilIdempotentAck(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("new bolt store: %v", err)
	}

	env := entities.Envelope{
		ID:             "offline-1",
		AgentID:        "agent-1",
		Kind:           "metric",
		SchemaVersion:  1,
		TSUnixMs:       time.Unix(100, 0).UnixMilli(),
		Endpoint:       "/v1/ingest/metrics",
		AuthHeader:     "Token secret",
		IdentityHeader: "identity-a",
		Body:           map[string]any{"value": 42},
	}
	if err := store.Push(env); err != nil {
		t.Fatalf("push envelope: %v", err)
	}
	firstRead, err := store.Peek(10)
	if err != nil {
		t.Fatalf("first peek: %v", err)
	}
	if len(firstRead) != 1 || firstRead[0].ID != env.ID || firstRead[0].Endpoint != env.Endpoint || firstRead[0].AuthHeader != env.AuthHeader || firstRead[0].IdentityHeader != env.IdentityHeader {
		t.Fatalf("unexpected first replay batch: %#v", firstRead)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("reopen bolt store: %v", err)
	}
	defer reopened.Close()

	replayed, err := reopened.Peek(10)
	if err != nil {
		t.Fatalf("replay peek: %v", err)
	}
	if !reflect.DeepEqual(firstRead, replayed) {
		t.Fatalf("expected durable replay until ack, got %#v want %#v", replayed, firstRead)
	}
	if err := reopened.Delete([]string{env.ID, env.ID, "unknown"}); err != nil {
		t.Fatalf("idempotent ack failed: %v", err)
	}
	if err := reopened.Delete([]string{env.ID}); err != nil {
		t.Fatalf("second ack should be idempotent: %v", err)
	}
	count, bytes := reopened.Len()
	if count != 0 || bytes != 0 {
		t.Fatalf("expected empty outbox after ack, got count=%d bytes=%d", count, bytes)
	}
}

func TestBoltStoreSimulates24hOfflineReplayWithoutDuplicatesAfterAck(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("new bolt store: %v", err)
	}
	defer store.Close()

	wantIDs := push24hOfflineReplay(t, store)

	firstReplay, err := store.Peek(24)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	secondReplay, err := store.Peek(24)
	if err != nil {
		t.Fatalf("second replay: %v", err)
	}
	if !reflect.DeepEqual(envelopeIDs(firstReplay), wantIDs) {
		t.Fatalf("unexpected first replay IDs: %#v", envelopeIDs(firstReplay))
	}
	if !reflect.DeepEqual(firstReplay, secondReplay) {
		t.Fatalf("expected stable replay until ack")
	}

	ackIDs := append(envelopeIDs(firstReplay), wantIDs[0], wantIDs[5], "unknown")
	if err := store.Delete(ackIDs); err != nil {
		t.Fatalf("ack duplicate IDs: %v", err)
	}
	if err := store.Delete(wantIDs); err != nil {
		t.Fatalf("second ack should be idempotent: %v", err)
	}
	afterAck, err := store.Peek(24)
	if err != nil {
		t.Fatalf("after ack replay: %v", err)
	}
	if len(afterAck) != 0 {
		t.Fatalf("expected no duplicates after ack, got %#v", envelopeIDs(afterAck))
	}
}

func TestBoltStorePreservesQueueAcrossCollectorRestart(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("new bolt store: %v", err)
	}

	beforeRestart := []entities.Envelope{
		restartEnvelope("restart-01-metrics", "/v1/ingest/metrics", 1),
		restartEnvelope("restart-02-health", "/v1/ingest/health", 2),
	}
	for _, env := range beforeRestart {
		if err := store.Push(env); err != nil {
			t.Fatalf("push before restart %s: %v", env.ID, err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close before restart: %v", err)
	}

	reopened, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatalf("reopen after restart: %v", err)
	}
	defer reopened.Close()
	afterRestart := restartEnvelope("restart-03-inventory", "/v1/ingest/inventory", 3)
	if err := reopened.Push(afterRestart); err != nil {
		t.Fatalf("push after restart: %v", err)
	}

	replayed, err := reopened.Peek(10)
	if err != nil {
		t.Fatalf("peek after restart: %v", err)
	}
	wantIDs := []string{"restart-01-metrics", "restart-02-health", "restart-03-inventory"}
	if !reflect.DeepEqual(envelopeIDs(replayed), wantIDs) {
		t.Fatalf("unexpected replay order after restart: got %#v want %#v", envelopeIDs(replayed), wantIDs)
	}
	if err := reopened.Delete([]string{"restart-01-metrics"}); err != nil {
		t.Fatalf("ack first item: %v", err)
	}
	remaining, err := reopened.Peek(10)
	if err != nil {
		t.Fatalf("peek remaining: %v", err)
	}
	wantRemaining := []string{"restart-02-health", "restart-03-inventory"}
	if !reflect.DeepEqual(envelopeIDs(remaining), wantRemaining) {
		t.Fatalf("unexpected remaining queue after partial ack: got %#v want %#v", envelopeIDs(remaining), wantRemaining)
	}
}

func push24hOfflineReplay(t *testing.T, store *BoltStore) []string {
	t.Helper()
	start := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	wantIDs := make([]string, 0, 24)
	for hour := 0; hour < 24; hour++ {
		id := fmt.Sprintf("offline-24h-%02d", hour)
		wantIDs = append(wantIDs, id)
		if err := store.Push(entities.Envelope{
			ID:            id,
			AgentID:       "agent-1",
			Kind:          "metric",
			SchemaVersion: 1,
			TSUnixMs:      start.Add(time.Duration(hour) * time.Hour).UnixMilli(),
			Endpoint:      "/v1/ingest/metrics",
			Body:          map[string]any{"hour": hour},
		}); err != nil {
			t.Fatalf("push offline envelope %d: %v", hour, err)
		}
	}
	return wantIDs
}

func restartEnvelope(id, endpoint string, seq int) entities.Envelope {
	return entities.Envelope{
		ID:            id,
		AgentID:       "agent-1",
		Kind:          "metric",
		SchemaVersion: 1,
		TSUnixMs:      time.Date(2026, 6, 18, 12, seq, 0, 0, time.UTC).UnixMilli(),
		Endpoint:      endpoint,
		Body:          map[string]any{"seq": seq},
	}
}

func envelopeIDs(list []entities.Envelope) []string {
	ids := make([]string, 0, len(list))
	for _, env := range list {
		ids = append(ids, env.ID)
	}
	return ids
}
