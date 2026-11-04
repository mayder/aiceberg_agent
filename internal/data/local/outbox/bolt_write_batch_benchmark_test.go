package outbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	bolt "go.etcd.io/bbolt"
)

func BenchmarkBoltDurableConcurrentBurst(b *testing.B) {
	for _, batch := range []bool{false, true} {
		b.Run(fmt.Sprintf("batch=%v", batch), func(b *testing.B) { benchmarkBoltBurst(b, batch) })
	}
}

func fragmentedBoltFixture(b *testing.B, batch bool) *BoltStore {
	b.Helper()
	s, err := NewBoltStoreWithOptions(b.TempDir()+"/synthetic.db", 256, BoltOptions{BatchWrites: batch})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(s.bucket)
		for i := 0; i < 24000; i++ {
			id := fmt.Sprintf("seed-%05d", i)
			raw, err := json.Marshal(storedEnvelope{Env: entities.Envelope{ID: id, Body: strings.Repeat("x", 4096)}})
			if err != nil {
				return err
			}
			if err = bucket.Put([]byte(id), raw); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	err = s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(s.bucket)
		for i := 64; i < 24000; i++ {
			if err := bucket.Delete([]byte(fmt.Sprintf("seed-%05d", i))); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		b.Fatal(err)
	}
	return s
}

func benchmarkBoltBurst(b *testing.B, batch bool) {
	s := fragmentedBoltFixture(b, batch)
	before := s.db.Stats()
	b.ResetTimer()
	for wave := 0; wave < b.N; wave++ {
		var wg sync.WaitGroup
		errs := make(chan error, 32)
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				id := fmt.Sprintf("burst-%d-%d", wave, i)
				err := s.Push(entities.Envelope{ID: id, Body: strings.Repeat("x", 4096)})
				if err == nil {
					err = s.Delete([]string{id})
				}
				errs <- err
			}(i)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				b.Fatal(err)
			}
		}
	}
	b.StopTimer()
	after := s.db.Stats()
	delta := after.Sub(&before)
	b.ReportMetric(float64(delta.TxStats.PageAlloc)/float64(b.N), "page-allocated-B/burst")
	b.ReportMetric(float64(delta.TxStats.Write)/float64(b.N), "writes/burst")
	if count, _ := s.Len(); count != 64 {
		b.Fatalf("retained fixture changed: %d", count)
	}
}
