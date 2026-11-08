package configfailure

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestThousandEqualCyclesSummaryAndRecovery(t *testing.T) {
	s := NewState()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC).UnixMilli()
	s.Failure("agent-1", "12", "hash", "hash", Cause{"disk_full", "persistent"}, now, now+60000, true)
	first := s.Pending[0]
	for cycle := int64(1); cycle <= 1000; cycle++ {
		s.Repeated(now + cycle)
	}
	if len(s.Pending) != 1 || s.Entries[0].Diagnostic.Count != 1001 || s.Entries[0].Diagnostic.Attempt != 1 {
		t.Fatalf("unexpected repeated state: %+v", s)
	}
	s.Repeated(now + SummaryInterval.Milliseconds())
	s.Repeated(now + SummaryInterval.Milliseconds() + 1)
	if len(s.Pending) != 2 || s.Pending[1].Diagnostic.Phase != "summary" {
		t.Fatal("missing or repeated summary")
	}
	if s.Pending[0] != first {
		t.Fatal("unacknowledged report was mutated")
	}
	if !s.Recover(now+SummaryInterval.Milliseconds()+2, true) {
		t.Fatal("missing recovery")
	}
	if s.Recover(now+SummaryInterval.Milliseconds()+3, true) {
		t.Fatal("duplicate recovery")
	}
	if len(s.Pending) != 3 || s.Pending[2].Diagnostic.Phase != "recovered" || s.Pending[2].Diagnostic.NextRetry != 0 {
		t.Fatal("bad recovery")
	}
	s.Acknowledge(first.Diagnostic.ReportID)
	s.Acknowledge(first.Diagnostic.ReportID)
	if len(s.Pending) != 2 {
		t.Fatal("ACK retry removed another report")
	}
}

func TestChangesRebootClockSkewAndSameMillisecondRecurrence(t *testing.T) {
	s := NewState()
	s.Failure("agent", "1", "hash", "hash", Cause{"unknown", "unknown"}, 1000, 61000, false)
	initial := s.Pending[0].Diagnostic.ReportID
	s.Recover(1000, false)
	s.Failure("agent", "1", "hash", "hash", Cause{"unknown", "unknown"}, 1000, 61000, false)
	if s.Pending[2].Diagnostic.ReportID == initial {
		t.Fatal("new episode reused original receipt")
	}
	s.Failure("agent", "2", "new-config", "new-config", Cause{"permission_denied", "persistent"}, 500, 60500, false)
	if s.Pending[3].Diagnostic.Phase != "changed" {
		t.Fatal("config change suppressed")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Pending) != 4 || restored.Pending[0].Diagnostic.ReportID != initial {
		t.Fatal("reboot lost ACK identity")
	}
	restored.Repeated(100)
	last := restored.Entries[len(restored.Entries)-1].Diagnostic
	if last.LastSeen < last.FirstSeen {
		t.Fatal("clock rollback broke ordering")
	}
}

func TestStateCapacityRetentionAndBoundedCounters(t *testing.T) {
	s := NewState()
	for i := 1; i <= 200; i++ {
		s.Failure("agent", fmt.Sprint(i), fmt.Sprint(i), fmt.Sprint(i), Cause{"unknown", "unknown"}, int64(i), 0, true)
	}
	if len(s.Entries) > MaxEntries || len(s.Pending) > MaxPending || s.Dropped == 0 {
		t.Fatal("unbounded state or silent capacity loss")
	}
	index := s.find(s.Active)
	s.Entries[index].Diagnostic.Count = MaxCount
	s.Repeated(1000)
	if s.Entries[index].Diagnostic.Count != MaxCount {
		t.Fatal("counter overflow")
	}
	s.Recover(1000, true)
	s.prune(1000 + ResolvedRetention.Milliseconds())
	for _, entry := range s.Entries {
		if entry.ResolvedAt != 0 {
			t.Fatal("resolved state retained beyond policy")
		}
	}
}
