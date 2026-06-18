package outbox

import (
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
