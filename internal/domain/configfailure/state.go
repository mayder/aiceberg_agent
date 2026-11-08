package configfailure

import (
	"fmt"
	"time"
)

const (
	MaxEntries        = 32
	MaxPending        = 64
	SummaryInterval   = 30 * time.Minute
	ResolvedRetention = 30 * 24 * time.Hour
)

// Delivery contains only bounded diagnostic facts and configuration identifiers.
// Its immutable report ID is retained until ACK so retries are not new incidents.
type Delivery struct {
	Version    string `json:"version"`
	ConfigHash string `json:"config_hash"`
	Diagnostic Report `json:"diagnostic"`
}

type Entry struct {
	Version        string `json:"version"`
	IdentityDigest string `json:"identity_digest"`
	ConfigHash     string `json:"config_hash"`
	Diagnostic     Report `json:"diagnostic"`
	LastQueued     int64  `json:"last_queued_ms"`
	ResolvedAt     int64  `json:"resolved_at_ms"`
}

type State struct {
	Sequence      uint64     `json:"sequence"`
	SchemaVersion int        `json:"schema_version"`
	Active        string     `json:"active"`
	Entries       []Entry    `json:"entries"`
	Pending       []Delivery `json:"pending"`
	Dropped       int64      `json:"dropped"`
}

func NewState() State { return State{SchemaVersion: SchemaVersion} }

// Failure records an actual apply attempt; polling suppression is recorded separately.
func (s *State) Failure(agent, version, digest, configHash string, cause Cause, now, next int64, durable bool) Report {
	fingerprint := Fingerprint(agent, "apply", digest, cause.Code)
	phase := "first"
	if s.Active != "" && s.Active != fingerprint {
		phase = "changed"
	}
	index := s.conditionIndex(fingerprint, version, digest, configHash, cause, now)
	entry := &s.Entries[index]
	entry.ConfigHash = configHash
	entry.Diagnostic.Count = saturate(entry.Diagnostic.Count)
	entry.Diagnostic.Attempt = saturate(entry.Diagnostic.Attempt)
	entry.Diagnostic.LastSeen = max(now, entry.Diagnostic.LastSeen)
	entry.Diagnostic.NextRetry = next
	entry.Diagnostic.Durable = durable
	s.Active = fingerprint
	if entry.LastQueued == 0 || phase == "changed" {
		s.queue(index, phase, now)
	} else {
		s.summary(index, now)
	}
	return s.Entries[index].Diagnostic
}

// Repeated increments observations without pretending an application was attempted.
func (s *State) Repeated(now int64) {
	index := s.find(s.Active)
	if index < 0 {
		return
	}
	s.Entries[index].Diagnostic.Count = saturate(s.Entries[index].Diagnostic.Count)
	s.Entries[index].Diagnostic.LastSeen = max(now, s.Entries[index].Diagnostic.LastSeen)
	s.summary(index, now)
}

func (s *State) Recover(now int64, durable bool) bool {
	index := s.find(s.Active)
	if index < 0 {
		return false
	}
	entry := &s.Entries[index]
	entry.Diagnostic.LastSeen = max(now, entry.Diagnostic.LastSeen)
	entry.Diagnostic.NextRetry = 0
	entry.Diagnostic.Durable = durable
	entry.ResolvedAt = entry.Diagnostic.LastSeen
	s.queue(index, "recovered", now)
	s.Active = ""
	s.prune(now)
	return true
}

func (s *State) Acknowledge(reportID string) {
	for i, item := range s.Pending {
		if item.Diagnostic.ReportID == reportID {
			s.Pending = append(s.Pending[:i], s.Pending[i+1:]...)
			return
		}
	}
}

func (s *State) find(fingerprint string) int {
	for i := range s.Entries {
		if s.Entries[i].Diagnostic.Fingerprint == fingerprint {
			return i
		}
	}
	return -1
}

func (s *State) summary(index int, now int64) {
	if now-s.Entries[index].LastQueued >= SummaryInterval.Milliseconds() {
		s.queue(index, "summary", now)
	}
}

func (s *State) queue(index int, phase string, now int64) {
	entry := &s.Entries[index]
	report := entry.Diagnostic
	report.Phase = phase
	s.Sequence++
	report.ReportID = Fingerprint(report.ID(), "report", fmt.Sprint(s.Sequence), "")
	if len(s.Pending) >= MaxPending {
		// Prefer evicting an unsent periodic summary, never allowing unbounded disk growth.
		remove := 0
		for i, item := range s.Pending {
			if item.Diagnostic.Phase == "summary" {
				remove = i
				break
			}
		}
		s.Pending = append(s.Pending[:remove], s.Pending[remove+1:]...)
		s.Dropped = saturate(s.Dropped)
	}
	s.Pending = append(s.Pending, Delivery{Version: entry.Version, ConfigHash: entry.ConfigHash, Diagnostic: report})
	entry.LastQueued = now
}

func (s *State) prune(now int64) {
	kept := s.Entries[:0]
	for _, entry := range s.Entries {
		if entry.ResolvedAt == 0 || now-entry.ResolvedAt < ResolvedRetention.Milliseconds() {
			kept = append(kept, entry)
		}
	}
	s.Entries = kept
}

func (s *State) evictOldest() {
	oldest := 0
	for i, entry := range s.Entries {
		if entry.Diagnostic.LastSeen < s.Entries[oldest].Diagnostic.LastSeen {
			oldest = i
		}
	}
	s.Entries = append(s.Entries[:oldest], s.Entries[oldest+1:]...)
}

func saturate(value int64) int64 {
	if value >= MaxCount {
		return MaxCount
	}
	return value + 1
}

// Current returns the active condition without exposing mutable internal slices.
func (s *State) Current() (Entry, bool) {
	index := s.find(s.Active)
	if index < 0 {
		return Entry{}, false
	}
	return s.Entries[index], true
}

// Expire removes only resolved diagnostic summaries, never pending delivery receipts.
func (s *State) Expire(now int64) bool {
	before := len(s.Entries)
	s.prune(now)
	return len(s.Entries) != before
}

func (s *State) conditionIndex(fingerprint, version, digest, configHash string, cause Cause, now int64) int {
	index := s.find(fingerprint)
	if index < 0 {
		s.prune(now)
		if len(s.Entries) >= MaxEntries {
			s.evictOldest()
		}
		s.Entries = append(s.Entries, Entry{Version: version, IdentityDigest: digest, ConfigHash: configHash, Diagnostic: Report{
			SchemaVersion: SchemaVersion, Fingerprint: fingerprint, Code: cause.Code, Kind: cause.Kind, FirstSeen: now,
		}})
		index = len(s.Entries) - 1
	} else if s.Entries[index].ResolvedAt != 0 {
		s.Entries[index].Diagnostic.Count = 0
		s.Entries[index].Diagnostic.Attempt = 0
		s.Entries[index].Diagnostic.FirstSeen = now
		s.Entries[index].LastQueued = 0
		s.Entries[index].ResolvedAt = 0
	}
	return index
}
