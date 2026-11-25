package agentless

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	bolt "go.etcd.io/bbolt"
)

func TestFencedDelegationRetainsExactResultOutsideTheOutbox(t *testing.T) {
	store, err := NewBoltStore(filepath.Join(t.TempDir(), "assignment.db"), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	observation := entities.AgentlessObservation{ID: "fixture-1", DedupeKey: "fixture-key", Status: "ok"}
	if err := store.Push(observation); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := store.db.View(func(tx *bolt.Tx) error {
		before = bytes.Clone(tx.Bucket(store.bucket).Get([]byte(observation.ID)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.RetainFencedDelegatedSNMP(observation.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Len(); n != 0 {
		t.Fatal("fenced result remained scheduled")
	}
	if err := store.db.View(func(tx *bolt.Tx) error {
		got := tx.Bucket([]byte("relay_assignment_fenced_v1")).Get([]byte(observation.ID))
		if !bytes.Equal(before, got) || len(got) == 0 {
			t.Fatal("original result was lost or modified")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
