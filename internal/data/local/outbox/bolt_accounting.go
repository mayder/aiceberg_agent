package outbox

import (
	"encoding/binary"

	bolt "go.etcd.io/bbolt"
)

var accountingBucket = []byte("outbox_accounting_v1")
var accountingKey = []byte("totals")

// Rebuilt once on every open, including after writes by an older binary.
// Values and capacity accounting commit in the same Bolt transaction.
func rebuildAccounting(tx *bolt.Tx, bucket []byte) error {
	count, size := bucketStats(tx, bucket)
	return writeAccounting(tx, count, size)
}

func writeAccounting(tx *bolt.Tx, count int, size int64) error {
	meta, err := tx.CreateBucketIfNotExists(accountingBucket)
	if err != nil {
		return err
	}
	var raw [16]byte
	binary.BigEndian.PutUint64(raw[:8], uint64(count))
	binary.BigEndian.PutUint64(raw[8:], uint64(size))
	return meta.Put(accountingKey, raw[:])
}

func accountedStats(tx *bolt.Tx, bucket []byte) (int, int64) {
	if meta := tx.Bucket(accountingBucket); meta != nil {
		if raw := meta.Get(accountingKey); len(raw) == 16 {
			return int(binary.BigEndian.Uint64(raw[:8])), int64(binary.BigEndian.Uint64(raw[8:]))
		}
	}
	return bucketStats(tx, bucket)
}
