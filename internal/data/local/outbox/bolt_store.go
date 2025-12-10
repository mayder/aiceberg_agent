package outbox

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

// BoltStore persistente com limites de tamanho e coleta simples por lote.
type BoltStore struct {
	path     string
	maxMB    int
	db       *bolt.DB
	mu       sync.Mutex
	bucket   []byte
	maxBytes int64
}

func NewBoltStore(path string, maxMB int) (*BoltStore, error) {
	if path == "" {
		return nil, errors.New("bolt path obrigatório")
	}
	if maxMB <= 0 {
		maxMB = 100
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	store := &BoltStore{path: path, maxMB: maxMB, maxBytes: maxBytes, db: db, bucket: []byte("outbox")}
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

func (b *BoltStore) Close() error {
	return b.db.Close()
}

func (b *BoltStore) Push(e entities.Envelope) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	key := []byte(e.ID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok, err := b.withinLimit(int64(len(raw))); err != nil {
		return err
	} else if !ok {
		return errors.New("outbox full (max MB reached)")
	}
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		return bk.Put(key, raw)
	})
}

// Peek retorna até n envelopes sem removê-los.
func (b *BoltStore) Peek(n int) ([]entities.Envelope, error) {
	var out []entities.Envelope
	if n <= 0 {
		return out, nil
	}
	err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		c := bk.Cursor()
		for k, v := c.First(); k != nil && len(out) < n; k, v = c.Next() {
			var env entities.Envelope
			if err := json.Unmarshal(v, &env); err != nil {
				continue
			}
			out = append(out, env)
		}
		return nil
	})
	return out, err
}

// Delete remove envelopes por ID.
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

// Len retorna contagem de itens e bytes totais aproximados.
func (b *BoltStore) Len() (int, int64) {
	var count int
	var bytes int64
	_ = b.db.View(func(tx *bolt.Tx) error {
		count, bytes = bucketStats(tx, b.bucket)
		return nil
	})
	return count, bytes
}

// GC apaga itens mais antigos até ficar abaixo do limite.
func (b *BoltStore) GC() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.db.Update(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		c := bk.Cursor()
		_, curBytes := bucketStats(tx, b.bucket)
		for k, v := c.First(); k != nil && curBytes > b.maxBytes; k, v = c.Next() {
			curBytes -= int64(len(v))
			_ = bk.Delete(k)
		}
		return nil
	})
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

func bucketStats(tx *bolt.Tx, bucket []byte) (count int, bytes int64) {
	bk := tx.Bucket(bucket)
	if bk == nil {
		return 0, 0
	}
	c := bk.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		count++
		bytes += int64(len(v))
	}
	return count, bytes
}
