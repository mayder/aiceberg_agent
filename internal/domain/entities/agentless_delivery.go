package entities

import (
	"fmt"
	"sort"
	"time"
)

// AgentlessDeliveryStream preserves each collection independently. Legacy items
// without collection identity conservatively share a check/segment stream.
func AgentlessDeliveryStream(o AgentlessObservation) string {
	kind := o.SegmentID
	if kind == "" {
		kind = o.CollectionKind
	}
	key := fmt.Sprintf("%d/%s", o.CheckID, kind)
	if o.SegmentStartedAt != nil && !o.SegmentStartedAt.IsZero() {
		return key + "/" + o.SegmentStartedAt.UTC().Format(time.RFC3339Nano)
	}
	if o.CorrelationID != "" {
		return key + "/" + o.CorrelationID
	}
	return key
}

func deliveryTime(o AgentlessObservation) time.Time {
	if !o.ObservedAt.IsZero() {
		return o.ObservedAt
	}
	return o.CreatedAt
}

func deliveryBefore(a, b AgentlessObservation) bool {
	ta, tb := deliveryTime(a), deliveryTime(b)
	if a.SegmentStartedAt != nil {
		ta = *a.SegmentStartedAt
	}
	if b.SegmentStartedAt != nil {
		tb = *b.SegmentStartedAt
	}
	if !ta.Equal(tb) {
		return ta.Before(tb)
	}
	if a.SegmentSeq > 0 && b.SegmentSeq > 0 && a.SegmentSeq != b.SegmentSeq {
		return a.SegmentSeq < b.SegmentSeq
	}
	if a.IsFinal != b.IsFinal {
		return !a.IsFinal
	}
	return a.ID < b.ID
}

// SelectAgentlessDelivery returns at most one head per stream. Three slots favor
// recent fast streams; every fourth serves the oldest eligible head, including
// legacy and slow streams. IDs and payloads remain unchanged.
func SelectAgentlessDelivery(list []AgentlessObservation, n int, ordinal uint64, excluded map[string]bool) []AgentlessObservation {
	heads := map[string]AgentlessObservation{}
	for _, o := range list {
		key := AgentlessDeliveryStream(o)
		if excluded[key] {
			continue
		}
		old, ok := heads[key]
		if !ok || deliveryBefore(o, old) {
			heads[key] = o
		}
	}
	candidates := make([]AgentlessObservation, 0, len(heads))
	for _, o := range heads {
		candidates = append(candidates, o)
	}
	sort.Slice(candidates, func(i, j int) bool { return deliveryBefore(candidates[i], candidates[j]) })
	fast := deliveryFastCandidates(candidates)
	out := make([]AgentlessObservation, 0)
	selected := map[string]bool{}
	oldestIndex, fastIndex := 0, 0
	for len(out) < n && len(out) < len(candidates) {
		for oldestIndex < len(candidates) && selected[candidates[oldestIndex].ID] {
			oldestIndex++
		}
		for fastIndex < len(fast) && selected[fast[fastIndex].ID] {
			fastIndex++
		}
		next := candidates[oldestIndex]
		if (ordinal+uint64(len(out)))%4 != 3 && fastIndex < len(fast) {
			next = fast[fastIndex]
		}
		selected[next.ID] = true
		out = append(out, next)
	}
	return out
}

func deliveryFastCandidates(candidates []AgentlessObservation) []AgentlessObservation {
	fast := make([]AgentlessObservation, 0)
	for _, o := range candidates {
		if o.CollectionKind == "metrics_fast" || o.SegmentID == "metrics_fast" {
			fast = append(fast, o)
		}
	}
	sort.Slice(fast, func(i, j int) bool {
		if !deliveryTime(fast[i]).Equal(deliveryTime(fast[j])) {
			return deliveryTime(fast[i]).After(deliveryTime(fast[j]))
		}
		return deliveryBefore(fast[i], fast[j])
	})
	return fast
}
