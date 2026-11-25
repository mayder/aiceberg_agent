package agentless

import (
	"crypto/sha256"
	"path/filepath"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func trapReplayFixture(id string, boots, seconds uint32, received int64) entities.SNMPTrapReplayProof {
	return entities.SNMPTrapReplayProof{Engine: sha256.Sum256([]byte("fixture-engine")), Digest: sha256.Sum256([]byte(id)),
		Boots: boots, EngineTime: seconds, ReceivedAt: received}
}

func openTrapReplayFixture(t *testing.T, path string) *BoltStore {
	t.Helper()
	store, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSNMPTrapReplayProofSurvivesAckAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trap.db")
	store := openTrapReplayFixture(t, path)
	first := trapReplayFixture("first", 7, 1000, 1780000000)
	if err := store.PushSNMPTrap(entities.AgentlessObservation{ID: "saved"}, &first); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete([]string{"saved"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = openTrapReplayFixture(t, path)
	for _, tt := range []struct {
		name  string
		proof entities.SNMPTrapReplayProof
		want  int
	}{
		{"replay", first, 0},
		{"prior boot", trapReplayFixture("oldboot", 6, 1001, first.ReceivedAt+1), 0},
		{"expired in this boot", trapReplayFixture("oldtime", 7, 900, first.ReceivedAt+100), 0},
		{"bounded reorder", trapReplayFixture("reordered", 7, 996, first.ReceivedAt+1), 1},
		{"legitimate reboot", trapReplayFixture("newboot", 8, 1, first.ReceivedAt+101), 2},
		{"replay after reboot", first, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := store.PushSNMPTrap(entities.AgentlessObservation{ID: tt.name}, &tt.proof); err != nil {
				t.Fatal(err)
			}
			assertAgentlessBoltUsage(t, store, tt.want)
		})
	}
}

func TestSNMPTrapCapacityFailureDoesNotConsumeReplayProof(t *testing.T) {
	store := openTrapReplayFixture(t, filepath.Join(t.TempDir(), "trap.db"))
	proof := trapReplayFixture("first", 1, 10, 1780000000)
	oversized := entities.AgentlessObservation{ID: "oversized", Payload: map[string]any{"fixture": strings.Repeat("x", 1<<20)}}
	if err := store.PushSNMPTrap(oversized, &proof); err == nil {
		t.Fatal("expected queue capacity failure")
	}
	assertAgentlessBoltUsage(t, store, 0)
	if err := store.PushSNMPTrap(entities.AgentlessObservation{ID: "retry"}, &proof); err != nil {
		t.Fatal(err)
	}
	pending, err := store.Peek(10)
	if err != nil || len(pending) != 1 || pending[0].ID != "retry" {
		t.Fatalf("notification was lost after rollback: %v", err)
	}
}
