package outbox

import (
	bolt "go.etcd.io/bbolt"
	"time"
)

const (
	boltWriteBatchSize  = 64
	boltWriteBatchDelay = 5 * time.Millisecond
)

// BoltOptions controls scheduling and decoding; successful writes remain durable.
type BoltOptions struct {
	BatchWrites bool
	// RebuildFreelist omits only the reconstructible free-page index. Data and
	// metadata still fsync on every successful transaction. Reopen scans pages.
	RebuildFreelist bool
	RawBodies       bool
}

// Batch callbacks only change their transaction: bbolt can retry them after a
// neighbouring callback fails. Return to the caller still waits for disk commit.
func (b *BoltStore) write(fn func(*bolt.Tx) error) error {
	if b.batchWrites {
		return b.db.Batch(fn)
	}
	return b.db.Update(fn)
}
