package agentless

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type BoltStore struct {
	path     string
	maxMB    int
	db       *bolt.DB
	mu       sync.Mutex
	bucket   []byte
	maxBytes int64
}

type storedObservation struct {
	Obs entities.AgentlessObservation `json:"obs"`
}

type storedObservationMeta struct {
	Obs struct {
		CreatedAt string `json:"created_at"`
	} `json:"obs"`
}

func NewBoltStore(path string, maxMB int) (*BoltStore, error) {
	if path == "" {
		return nil, errors.New("bolt path obrigatório")
	}
	if maxMB <= 0 {
		maxMB = 50
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	store := &BoltStore{path: path, maxMB: maxMB, maxBytes: maxBytes, db: db, bucket: []byte("agentless_outbox")}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (b *BoltStore) init() error {
	return b.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(b.bucket)
		return err
	})
}

func (b *BoltStore) Push(o entities.AgentlessObservation) error {
	raw, err := json.Marshal(storedObservation{Obs: o})
	if err != nil {
		return err
	}
	key := []byte(o.ID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok, err := b.withinLimit(int64(len(raw))); err != nil {
		return err
	} else if !ok {
		return errors.New("agentless outbox full (max MB reached)")
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		return bk.Put(key, raw)
	})
}

func (b *BoltStore) Peek(n int) ([]entities.AgentlessObservation, error) {
	var out []entities.AgentlessObservation
	if n <= 0 {
		return out, nil
	}
	err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		c := bk.Cursor()
		for k, v := c.First(); k != nil && len(out) < n; k, v = c.Next() {
			var stored storedObservation
			if err := json.Unmarshal(v, &stored); err != nil {
				continue
			}
			out = append(out, stored.Obs)
		}
		return nil
	})
	return out, err
}

func (b *BoltStore) Delete(ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	idset := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idset[id] = struct{}{}
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		for id := range idset {
			_ = bk.Delete([]byte(id))
		}
		return nil
	})
}

func (b *BoltStore) Len() (int, int64) {
	var count int
	var bytes int64
	_ = b.db.View(func(tx *bolt.Tx) error {
		count, bytes = bucketStats(tx, b.bucket)
		return nil
	})
	return count, bytes
}

func (b *BoltStore) withinLimit(nextBytes int64) (bool, error) {
	var curBytes int64
	err := b.db.View(func(tx *bolt.Tx) error {
		_, curBytes = bucketStats(tx, b.bucket)
		return nil
	})
	if err != nil {
		return false, err
	}
	return curBytes+nextBytes <= b.maxBytes, nil
}

func (b *BoltStore) Prune(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}
	removed := 0
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	_ = b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		c := bk.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var stored storedObservationMeta
			if err := json.Unmarshal(v, &stored); err != nil {
				continue
			}
			created, err := time.Parse(time.RFC3339Nano, stored.Obs.CreatedAt)
			if err != nil {
				continue
			}
			if now.Sub(created) > maxAge {
				_ = c.Delete()
				removed++
			}
		}
		return nil
	})
	return removed, nil
}

func bucketStats(tx *bolt.Tx, bucket []byte) (count int, bytes int64) {
	bk := tx.Bucket(bucket)
	if bk == nil {
		return 0, 0
	}
	_ = bk.ForEach(func(_, v []byte) error {
		count++
		bytes += int64(len(v))
		return nil
	})
	return count, bytes
}
