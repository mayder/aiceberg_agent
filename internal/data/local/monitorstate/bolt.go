package monitorstate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	bolt "go.etcd.io/bbolt"
)

const scopeBytes = sha256.Size * 2

var (
	bucket        = []byte("monitoring_pipeline_state_v1")
	slot          = []byte("checkpoint")
	errCheckpoint = errors.New("pipeline_checkpoint_invalid")
	errNoWrite    = errors.New("pipeline_checkpoint_read_only")
)

// BoltStore owns a dedicated local database for one pipeline binding.
type BoltStore struct {
	db     *bolt.DB
	writer chan struct{}
}

// Create provisions an empty checkpoint. It fails if the path already exists.
func Create(path string) (*BoltStore, error) {
	return open(path, true)
}

// Open recovers a provisioned checkpoint. A missing file never becomes empty state.
func Open(path string) (*BoltStore, error) {
	return open(path, false)
}

func open(path string, create bool) (*BoltStore, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{
		Timeout: 150 * time.Millisecond,
		OpenFile: func(path string, flags int, mode os.FileMode) (*os.File, error) {
			return openFile(path, flags, mode, create)
		},
	})
	if err != nil {
		return nil, errors.New("pipeline_checkpoint_open_failed")
	}
	store := &BoltStore{db: db, writer: make(chan struct{}, 1)}
	if create {
		err = db.Update(initialize)
	} else {
		err = db.View(func(tx *bolt.Tx) error {
			_, _, err := read(tx)
			return err
		})
	}
	if err != nil {
		_ = db.Close()
		return nil, errCheckpoint
	}
	return store, nil
}

func openFile(path string, flags int, mode os.FileMode, create bool) (*os.File, error) {
	if create {
		return os.OpenFile(path, flags|os.O_EXCL, mode)
	}
	file, err := os.OpenFile(path, flags&^os.O_CREATE, mode)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 ||
		(runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		_ = file.Close()
		return nil, errCheckpoint
	}
	return file, nil
}

func initialize(tx *bolt.Tx) error {
	dest, err := tx.CreateBucket(bucket)
	if err != nil {
		return err
	}
	return dest.Put(slot, frame(make([]byte, scopeBytes), nil))
}

func (s *BoltStore) Close() error {
	return s.db.Close()
}

func (s *BoltStore) UpdatePipelineState(ctx context.Context, scope string, update func([]byte) ([]byte, error)) error {
	if s == nil || len(scope) != scopeBytes {
		return errCheckpoint
	}
	if _, err := hex.DecodeString(scope); err != nil {
		return errCheckpoint
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.writer <- struct{}{}:
		defer func() { <-s.writer }()
	}
	err := s.db.Update(func(tx *bolt.Tx) error {
		return updateTransaction(ctx, tx, scope, update)
	})
	if err == errNoWrite {
		return nil
	}
	return err
}

func updateTransaction(ctx context.Context, tx *bolt.Tx, scope string, update func([]byte) ([]byte, error)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	bound, previous, err := read(tx)
	if err != nil {
		return err
	}
	if previous != nil && string(bound) != scope {
		return errors.New("pipeline_checkpoint_scope_mismatch")
	}
	next, err := update(bytes.Clone(previous))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if next == nil {
		return errNoWrite
	}
	if len(next) == 0 || len(next) > monitoring.MaxPipelineCheckpointBytes {
		return errCheckpoint
	}
	return tx.Bucket(bucket).Put(slot, frame([]byte(scope), next))
}

func read(tx *bolt.Tx) ([]byte, []byte, error) {
	source := tx.Bucket(bucket)
	if source == nil {
		return nil, nil, errCheckpoint
	}
	raw := source.Get(slot)
	if len(raw) < sha256.Size+scopeBytes || len(raw) > sha256.Size+scopeBytes+monitoring.MaxPipelineCheckpointBytes {
		return nil, nil, errCheckpoint
	}
	payload := raw[sha256.Size:]
	digest := sha256.Sum256(payload)
	if !bytes.Equal(raw[:sha256.Size], digest[:]) {
		return nil, nil, errCheckpoint
	}
	scope, state := payload[:scopeBytes], payload[scopeBytes:]
	if len(state) == 0 {
		if !bytes.Equal(scope, make([]byte, scopeBytes)) {
			return nil, nil, errCheckpoint
		}
		state = nil
	} else if _, err := hex.DecodeString(string(scope)); err != nil {
		return nil, nil, errCheckpoint
	}
	return scope, state, nil
}

func frame(scope, state []byte) []byte {
	raw := make([]byte, sha256.Size+len(scope)+len(state))
	copy(raw[sha256.Size:], scope)
	copy(raw[sha256.Size+len(scope):], state)
	digest := sha256.Sum256(raw[sha256.Size:])
	copy(raw, digest[:])
	return raw
}
