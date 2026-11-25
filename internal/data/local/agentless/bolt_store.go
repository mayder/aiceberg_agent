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
	path           string
	db             *bolt.DB
	mu             sync.Mutex
	bucket         []byte
	maxBytes       int64
	minRecordBytes int64
	// Usage is rebuilt on open and changed only after a Bolt commit.
	items     int
	usedBytes int64
	closed    bool
}

type storedObservation struct {
	Obs entities.AgentlessObservation `json:"obs"`
}

var ErrOutboxFull = errors.New("agentless outbox full (max MB reached)")

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
	minRecord, err := json.Marshal(storedObservation{})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	store := &BoltStore{path: path, maxBytes: maxBytes, minRecordBytes: int64(len(minRecord)), db: db, bucket: []byte("agentless_outbox")}
	if err := store.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (b *BoltStore) init() error {
	return b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(b.bucket)
		if err != nil {
			return err
		}
		return bucket.ForEach(func(_, raw []byte) error {
			b.items++
			b.usedBytes += int64(len(raw))
			return nil
		})
	})
}

func (b *BoltStore) Push(o entities.AgentlessObservation) error {
	return b.push(o, nil)
}

func (b *BoltStore) push(o entities.AgentlessObservation, proof *entities.SNMPTrapReplayProof) error {
	raw, err := json.Marshal(storedObservation{Obs: o})
	if err != nil {
		return err
	}
	key := []byte(o.ID)
	b.mu.Lock()
	defer b.mu.Unlock()
	var added int
	var bytesDelta int64
	err = b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		old := bucket.Get(key)
		delta := int64(len(raw) - len(old))
		if delta > 0 && delta > b.maxBytes-b.usedBytes {
			return ErrOutboxFull
		}
		if proof != nil {
			accepted, err := admitSNMPTrap(tx, *proof)
			if err != nil || !accepted {
				return err
			}
		}
		if old == nil {
			added = 1
		}
		bytesDelta = delta
		return bucket.Put(key, raw)
	})
	if err == nil {
		b.items += added
		b.usedBytes += bytesDelta
	}
	return err
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
	b.mu.Lock()
	defer b.mu.Unlock()
	var removed int
	var removedBytes int64
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(b.bucket)
		for _, id := range ids {
			key := []byte(id)
			old := bucket.Get(key)
			if old == nil {
				continue
			}
			removed++
			removedBytes += int64(len(old))
			if err := bucket.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		b.items -= removed
		b.usedBytes -= removedBytes
	}
	return err
}

func (b *BoltStore) Len() (int, int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, 0
	}
	return b.items, b.usedBytes
}

func (b *BoltStore) AdmissionState() (int, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return 0, 0, bolt.ErrDatabaseNotOpen
	}
	return b.items, max(0, b.maxBytes-b.usedBytes) / b.minRecordBytes, nil
}

func (b *BoltStore) Prune(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil
	}
	removed := 0
	var removedBytes int64
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	err := b.db.Update(func(tx *bolt.Tx) error {
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
				removedBytes += int64(len(v))
				if err := c.Delete(); err != nil {
					return err
				}
				removed++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	b.items -= removed
	b.usedBytes -= removedBytes
	return removed, nil
}

// Project scheduling metadata only; decode payloads solely for selected heads.
// The scan is bounded by the configured outbox capacity, without a new index/schema.
func (b *BoltStore) PeekDelivery(n int, ordinal uint64, excluded map[string]bool) ([]entities.AgentlessObservation, error) {
	var out []entities.AgentlessObservation
	err := b.db.View(func(tx *bolt.Tx) error {
		bk := tx.Bucket(b.bucket)
		var candidates []entities.AgentlessObservation
		err := bk.ForEach(func(_, raw []byte) error {
			var projected struct {
				Obs struct {
					ID               string     `json:"id"`
					CheckID          int        `json:"check_id"`
					CollectionKind   string     `json:"snmp_collection_kind"`
					SegmentID        string     `json:"segment_id"`
					SegmentSeq       int        `json:"segment_seq"`
					IsFinal          bool       `json:"is_final"`
					CorrelationID    string     `json:"correlation_id"`
					ObservedAt       time.Time  `json:"observed_at"`
					CreatedAt        time.Time  `json:"created_at"`
					SegmentStartedAt *time.Time `json:"segment_started_at"`
				} `json:"obs"`
			}
			if err := json.Unmarshal(raw, &projected); err != nil {
				return err
			}
			o := projected.Obs
			candidates = append(candidates, entities.AgentlessObservation{ID: o.ID, CheckID: o.CheckID,
				CollectionKind: o.CollectionKind, SegmentID: o.SegmentID, SegmentSeq: o.SegmentSeq,
				IsFinal: o.IsFinal, CorrelationID: o.CorrelationID, ObservedAt: o.ObservedAt,
				CreatedAt: o.CreatedAt, SegmentStartedAt: o.SegmentStartedAt})
			return nil
		})
		if err != nil {
			return err
		}
		for _, head := range entities.SelectAgentlessDelivery(candidates, n, ordinal, excluded) {
			var stored storedObservation
			if err := json.Unmarshal(bk.Get([]byte(head.ID)), &stored); err != nil {
				return err
			}
			out = append(out, stored.Obs)
		}
		return nil
	})
	return out, err
}

// Close flushes the dedicated store; callers stop its worker first.
func (b *BoltStore) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return b.db.Close()
}
