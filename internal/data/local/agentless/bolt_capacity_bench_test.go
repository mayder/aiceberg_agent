package agentless

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	bolt "go.etcd.io/bbolt"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func BenchmarkAgentlessBoltCapacity(b *testing.B) {
	for _, pending := range []int{400, 10000} {
		b.Run(fmt.Sprintf("pending=%d", pending), func(b *testing.B) {
			store := seedAgentlessBoltBenchmark(b, pending)
			b.Run("admission", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, _, err := store.AdmissionState(); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("replace", func(b *testing.B) {
				entry := entities.AgentlessObservation{ID: "fixture-00000", CheckID: 1, Status: "ok"}
				b.ReportAllocs()
				for b.Loop() {
					if err := store.Push(entry); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func seedAgentlessBoltBenchmark(b *testing.B, pending int) *BoltStore {
	b.Helper()
	path := filepath.Join(b.TempDir(), "admission.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		b.Fatal(err)
	}
	seedErr := db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("agentless_outbox"))
		if err != nil {
			return err
		}
		for i := range pending {
			entry := entities.AgentlessObservation{ID: fmt.Sprintf("fixture-%05d", i), CheckID: 1, Status: "ok"}
			raw, err := json.Marshal(storedObservation{Obs: entry})
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(entry.ID), raw); err != nil {
				return err
			}
		}
		return nil
	})
	closeErr := db.Close()
	if seedErr != nil || closeErr != nil {
		b.Fatalf("seed queue: %v %v", seedErr, closeErr)
	}
	store, err := NewBoltStore(path, 64)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	return store
}
