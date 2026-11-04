package outbox

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	bolt "go.etcd.io/bbolt"
)

func TestBoltFreelistRebuildSurvivesKillAndLegacyReopen(t *testing.T) {
	path := t.TempDir() + "/synthetic.db"
	killCommittedBatchWriter(t, path, "AICEBERG_BOLT_REBUILD_FREELIST=1")
	old, err := NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	got, err := old.Peek(200)
	if err != nil || len(got) != 128 {
		t.Fatalf("legacy recovery: records=%d err=%v", len(got), err)
	}
	for _, env := range got {
		if body, ok := env.Body.(map[string]any); !ok || body["value"] != strings.Repeat("x", 1024) {
			t.Fatal("body changed during recovery")
		}
		if env.AgentID != "synthetic" || env.AuthHeader != "Token synthetic" || env.IdentityHeader != "synthetic-identity" || env.Endpoint != "/v1/ingest/metrics" {
			t.Fatal("identity changed during recovery")
		}
	}
	if err := old.db.View(func(tx *bolt.Tx) error {
		for err := range tx.Check() {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := old.Push(entities.Envelope{ID: "after-rollback", Body: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := old.Len(); n != 129 {
		t.Fatal("legacy writer lost data")
	}
}

func TestBoltFreelistRebuildKeepsDataSyncAndLegacyContent(t *testing.T) {
	path := t.TempDir() + "/synthetic.db"
	old, err := NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := batchBurst(old, 128); err != nil {
		t.Fatal(err)
	}
	before, err := old.Peek(200)
	if err != nil {
		t.Fatal(err)
	}
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewBoltStoreWithOptions(path, 2, BoltOptions{BatchWrites: true, RebuildFreelist: true})
	if err != nil {
		t.Fatal(err)
	}
	if next.db.NoSync || !next.db.NoFreelistSync {
		t.Fatal("incorrect durability options")
	}
	if err := next.Push(entities.Envelope{ID: "new", Body: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	rollback, err := NewBoltStore(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Close()
	after, err := rollback.Peek(200)
	if err != nil || len(after) != len(before)+1 {
		t.Fatal("upgrade/rollback lost records")
	}
	for _, env := range before {
		found := false
		for _, got := range after {
			if reflect.DeepEqual(got, env) {
				found = true
			}
		}
		if !found {
			t.Fatal("retained record changed")
		}
	}
}

func TestBoltFreelistRebuildReopensFragmentedFileWithLegacyWriter(t *testing.T) {
	seeded := fragmentedBoltFixture(t, true)
	path := seeded.path
	if err := seeded.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewBoltStoreWithOptions(path, 256, BoltOptions{BatchWrites: true, RebuildFreelist: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Push(entities.Envelope{ID: "after-drain", Body: "synthetic"}); err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	old, err := NewBoltStore(path, 256)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	t.Logf("legacy recovery of fragmented fixture: %s", time.Since(started))
	if n, _ := old.Len(); n != 65 {
		t.Fatalf("recovery changed retained envelopes: %d", n)
	}
}
