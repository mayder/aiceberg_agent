package agentless

import (
	"encoding/binary"
	"encoding/hex"
	"errors"

	bolt "go.etcd.io/bbolt"
)

const maxAssignmentReservations = 4096
const maxFencedAssignmentBytes = 8 * 1024 * 1024

// RetainFencedDelegatedSNMP atomically moves the exact outbox bytes into the
// fenced bucket. It never recognizes a stale receipt as an accepted observation.
func (b *BoltStore) RetainFencedDelegatedSNMP(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	moved := 0
	err := b.db.Update(func(tx *bolt.Tx) error {
		outbox := tx.Bucket(b.bucket)
		key := []byte(id)
		raw := outbox.Get(key)
		if raw == nil {
			return nil
		}
		history, err := tx.CreateBucketIfNotExists([]byte("relay_assignment_fenced_v1"))
		if err != nil {
			return err
		}
		total := len(raw)
		if err := history.ForEach(func(_, value []byte) error { total += len(value); return nil }); err != nil {
			return err
		}
		if total > maxFencedAssignmentBytes {
			return errors.New("fenced relay assignment storage full")
		}
		if err := history.Put(key, raw); err != nil {
			return err
		}
		moved = len(raw)
		return outbox.Delete(key)
	})
	if err == nil && moved > 0 {
		b.items--
		b.usedBytes -= int64(moved)
	}
	return err
}

// ReserveDelegatedSNMP commits a replay reservation before the UDP request.
// Only a digest and expiry are stored, so a restart cannot repeat a live job.
func (b *BoltStore) ReserveDelegatedSNMP(digest string, expiresMS, nowMS int64) (bool, error) {
	if len(digest) != 64 || nowMS <= 0 || expiresMS <= nowMS {
		return false, errors.New("invalid relay assignment reservation")
	}
	key, err := hex.DecodeString(digest)
	if err != nil {
		return false, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	reserved := false
	err = b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("relay_assignment_reservations_v1"))
		if err != nil {
			return err
		}
		cursor := bucket.Cursor()
		retained := 0
		for k, value := cursor.First(); k != nil; k, value = cursor.Next() {
			if len(value) != 8 {
				return errors.New("invalid stored relay assignment reservation")
			}
			if binary.BigEndian.Uint64(value) <= uint64(nowMS) {
				if err := cursor.Delete(); err != nil {
					return err
				}
				continue
			}
			retained++
		}
		if bucket.Get(key) != nil {
			return nil
		}
		if retained >= maxAssignmentReservations {
			return errors.New("relay assignment reservation limit")
		}
		var expires [8]byte
		binary.BigEndian.PutUint64(expires[:], uint64(expiresMS))
		reserved = true
		return bucket.Put(key, expires[:])
	})
	return reserved && err == nil, err
}
