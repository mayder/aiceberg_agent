package outbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	bolt "go.etcd.io/bbolt"
)

func assertAccounting(t *testing.T, store *BoltStore) {
	t.Helper()
	var count int
	var size int64
	if err := store.db.View(func(tx *bolt.Tx) error {
		count, size = bucketStats(tx, store.bucket)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if gotCount, gotSize := store.Len(); gotCount != count || gotSize != size {
		t.Fatalf("accounting drift: got %d/%d, actual %d/%d", gotCount, gotSize, count, size)
	}
}

func TestBoltAccountingMutationsAndCapacityRollback(t *testing.T) {
	store, err := NewBoltStore(t.TempDir()+"/outbox.db", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	push := func(id, text string) {
		t.Helper()
		if err := store.Push(entities.Envelope{ID: id, AgentID: "fixture", TSUnixMs: time.Now().UnixMilli(), Body: map[string]any{"text": text}}); err != nil {
			t.Fatal(err)
		}
		assertAccounting(t, store)
	}
	push("a", "first")
	push("a", "replacement with different length")
	push("part", "existing part")
	if err := store.Replace("a", []entities.Envelope{{ID: "part", Body: map[string]any{"text": "new"}}, {ID: "new"}}); err != nil {
		t.Fatal(err)
	}
	assertAccounting(t, store)
	beforeCount, beforeSize := store.Len()
	if err := store.Push(entities.Envelope{ID: "large", Body: strings.Repeat("x", 2*1024*1024)}); err == nil {
		t.Fatal("capacity must reject without changing totals")
	}
	if n, size := store.Len(); n != beforeCount || size != beforeSize {
		t.Fatal("failed write changed totals")
	}
	if err := store.Delete([]string{"part", "part", "unknown"}); err != nil {
		t.Fatal(err)
	}
	assertAccounting(t, store)
	push("recent-1", "one")
	push("recent-2", "two")
	if _, err := store.Prune(PruneOptions{MaxPerAgent: 1}); err != nil {
		t.Fatal(err)
	}
	assertAccounting(t, store)
}

func TestBoltAccountingRebuiltAfterLegacyWriterAndReopen(t *testing.T) {
	path := t.TempDir() + "/outbox.db"
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(storedEnvelope{Env: entities.Envelope{ID: "legacy", Body: json.RawMessage(`{"retained":true}`)}})
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("outbox")).Put([]byte("legacy"), raw)
	}); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertAccounting(t, store)
	if n, _ := store.Len(); n != 1 {
		t.Fatal("rollback/upgrade lost the legacy envelope")
	}
}

func TestBoltAccountingConcurrentDurableBatch(t *testing.T) {
	store, err := NewBoltStoreWithOptions(t.TempDir()+"/outbox.db", 1, BoltOptions{BatchWrites: true})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Push(entities.Envelope{ID: fmt.Sprint(i), Body: json.RawMessage(`{}`)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	assertAccounting(t, store)
	if n, _ := store.Len(); n != 64 {
		t.Fatal("concurrent writes lost")
	}
}

func BenchmarkBoltAccountingFullQueue(b *testing.B) {
	store, err := NewBoltStore(b.TempDir()+"/outbox.db", 100)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	if err := store.db.Update(func(tx *bolt.Tx) error {
		raw := []byte(strings.Repeat("x", 1024))
		for i := 0; i < 12000; i++ {
			if err := tx.Bucket(store.bucket).Put([]byte(fmt.Sprintf("%08d", i)), raw); err != nil {
				return err
			}
		}
		return rebuildAccounting(tx, store.bucket)
	}); err != nil {
		b.Fatal(err)
	}
	b.Run("previous_full_scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = store.db.View(func(tx *bolt.Tx) error { bucketStats(tx, store.bucket); return nil })
		}
	})
	b.Run("transactional_totals", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			store.Len()
		}
	})
}
