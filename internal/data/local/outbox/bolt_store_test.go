package outbox

import (
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
