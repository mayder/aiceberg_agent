package agentless

import (
	"fmt"
	"github.com/you/aiceberg_agent/internal/domain/entities"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDeliverySelectionParityRestartSequenceAndFairness(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	list := []entities.AgentlessObservation{
		{ID: "z-part", CheckID: 1, CollectionKind: "metrics_fast", ObservedAt: at, SegmentStartedAt: &at, SegmentSeq: 1, DedupeKey: "part"},
		{ID: "a-final", CheckID: 1, CollectionKind: "metrics_fast", ObservedAt: at, SegmentStartedAt: &at, SegmentSeq: 2, IsFinal: true, DedupeKey: "final"},
		{ID: "slow-old", CheckID: 2, CollectionKind: "switchport_slow", ObservedAt: at.Add(-time.Hour)},
	}
	for i := 3; i < 9; i++ {
		list = append(list, entities.AgentlessObservation{ID: fmt.Sprint(i), CheckID: i, CollectionKind: "metrics_fast", ObservedAt: at.Add(time.Duration(i) * time.Second)})
	}
	mem := NewMemStore()
	path := filepath.Join(t.TempDir(), "outbox.db")
	bolt, err := NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range list {
		o.Payload = map[string]any{"unchanged": o.ID}
		if err := mem.Push(o); err != nil {
			t.Fatal(err)
		}
		if err := bolt.Push(o); err != nil {
			t.Fatal(err)
		}
	}
	if err := bolt.db.Close(); err != nil {
		t.Fatal(err)
	}
	bolt, err = NewBoltStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer bolt.db.Close()
	for ordinal := uint64(0); ordinal < 8; ordinal++ {
		a, err := mem.PeekDelivery(1, ordinal, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, err := bolt.PeekDelivery(1, ordinal, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("parity: %#v != %#v", a, b)
		}
		if ordinal == 3 && a[0].ID != "slow-old" {
			t.Fatalf("slow starvation: %s", a[0].ID)
		}
		if a[0].ID == "a-final" {
			t.Fatal("final overtook partial")
		}
	}
	all, _ := bolt.PeekDelivery(50, 0, map[string]bool{"2/switchport_slow": true})
	for _, o := range all {
		if o.ID == "a-final" || o.ID == "slow-old" {
			t.Fatal("sequence/exclusion violated")
		}
	}
	if len(all) != 7 {
		t.Fatalf("heads: %d", len(all))
	}
	if err := bolt.Delete([]string{"z-part"}); err != nil {
		t.Fatal(err)
	}
	all, _ = bolt.PeekDelivery(50, 0, nil)
	found := false
	for _, o := range all {
		if o.ID == "a-final" {
			found = true
		}
	}
	if !found {
		t.Fatal("final never became eligible")
	}
	count, _ := bolt.Len()
	if count != len(list)-1 {
		t.Fatal("read removed data")
	}
}

func BenchmarkDeliverySelectionBoltMixedBacklog(b *testing.B) {
	store, err := NewBoltStore(filepath.Join(b.TempDir(), "backlog.db"), 50)
	if err != nil {
		b.Fatal(err)
	}
	defer store.db.Close()
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 1000; i++ {
		kind := "metrics_fast"
		if i%2 == 0 {
			kind = "switchport_slow"
		}
		if err := store.Push(entities.AgentlessObservation{ID: fmt.Sprint(i), CheckID: i, CollectionKind: kind, ObservedAt: at, Payload: map[string]any{"data": strings.Repeat("x", 1024)}}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.PeekDelivery(100, uint64(i), nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDeliveryRecentCollectionCanPassOlderRunWithoutReorderingParts(t *testing.T) {
	old := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	recent := old.Add(time.Minute)
	list := []entities.AgentlessObservation{
		{ID: "old-part", CheckID: 7, CollectionKind: "metrics_fast", SegmentStartedAt: &old, ObservedAt: old, SegmentSeq: 1},
		{ID: "old-final", CheckID: 7, CollectionKind: "metrics_fast", SegmentStartedAt: &old, ObservedAt: old, SegmentSeq: 2, IsFinal: true},
		{ID: "new-part", CheckID: 7, CollectionKind: "metrics_fast", SegmentStartedAt: &recent, ObservedAt: recent, SegmentSeq: 1},
		{ID: "new-final", CheckID: 7, CollectionKind: "metrics_fast", SegmentStartedAt: &recent, ObservedAt: recent, SegmentSeq: 2, IsFinal: true},
	}
	fast := entities.SelectAgentlessDelivery(list, 1, 0, nil)
	if len(fast) != 1 || fast[0].ID != "new-part" {
		t.Fatal("fresh collection held behind old backlog")
	}
	oldest := entities.SelectAgentlessDelivery(list, 1, 3, nil)
	if len(oldest) != 1 || oldest[0].ID != "old-part" {
		t.Fatal("old collection starved or final overtook partial")
	}
}
