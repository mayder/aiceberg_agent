package agentless

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestAgentlessBoltUsageMatchesCommittedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentless.db")
	store := openAgentlessCapacityFixture(t, path)
	now := time.Now()
	for _, id := range []string{"current", "expired-a", "expired-b", "acked"} {
		created := now
		if strings.HasPrefix(id, "expired-") {
			created = now.Add(-2 * time.Hour)
		}
		if err := store.Push(entities.AgentlessObservation{ID: id, Status: "ok", CreatedAt: created}); err != nil {
			t.Fatal(err)
		}
	}
	assertAgentlessBoltUsage(t, store, 4)

	replacement := entities.AgentlessObservation{ID: "current", Status: "ok", CreatedAt: now, Message: "updated"}
	if err := store.Push(replacement); err != nil {
		t.Fatal(err)
	}
	assertAgentlessBoltUsage(t, store, 4)
	replacement.Message = strings.Repeat("x", 1<<20)
	if err := store.Push(replacement); !errors.Is(err, ErrOutboxFull) {
		t.Fatalf("expected capacity rejection: %v", err)
	}
	assertAgentlessBoltUsage(t, store, 4)

	if err := store.Delete([]string{"acked", "missing", "acked"}); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Prune(time.Hour); err != nil || removed != 2 {
		t.Fatalf("expected both expired records to be pruned: count=%d err=%v", removed, err)
	}
	assertAgentlessBoltUsage(t, store, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AdmissionState(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("closed queue reported capacity: %v", err)
	}

	reopened := openAgentlessCapacityFixture(t, path)
	assertAgentlessBoltUsage(t, reopened, 1)
	items, err := reopened.Peek(2)
	if err != nil || len(items) != 1 || items[0].Message != "updated" {
		t.Fatal("rollback or reopen altered the retained observation", err)
	}
}

func openAgentlessCapacityFixture(t *testing.T, path string) *BoltStore {
	t.Helper()
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func assertAgentlessBoltUsage(t *testing.T, store *BoltStore, expected int) {
	t.Helper()
	var count int
	var used int64
	if err := store.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(store.bucket).ForEach(func(_, raw []byte) error {
			count++
			used += int64(len(raw))
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	items, bytes := store.Len()
	if count != expected || items != count || bytes != used {
		t.Fatalf("queue usage differs from committed Bolt records: items=%d/%d bytes=%d/%d", items, count, bytes, used)
	}
	items, slots, err := store.AdmissionState()
	if err != nil || items != expected || slots != (store.maxBytes-used)/store.minRecordBytes {
		t.Fatalf("admission disagrees with committed usage: items=%d slots=%d err=%v", items, slots, err)
	}
}
