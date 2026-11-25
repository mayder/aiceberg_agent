package logdiscovery

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
	"github.com/you/aiceberg_agent/internal/platform/collectors/serviceretirement"
)

const (
	retirementGrace      = 2 * time.Minute
	retirementFreshness  = 300
	maxTrackedLogSources = 1000
	maxRetirements       = 16
)

type SourceRetirement struct {
	client, asset int64
	sources       map[string]*trackedLogSource
	probeChannel  func(string) (channelPresence, error)
	probeSystemd  func(context.Context, string) (bool, error)
	resumeAfter   string
}

type trackedLogSource struct {
	absent      func(context.Context) bool
	lastSeen    time.Time
	absentSince time.Time
	event       monitoring.DiscoveryEvent
}

func NewSourceRetirement(client, asset int64) *SourceRetirement {
	probe, _ := serviceretirement.SystemdAbsenceProbe()
	return &SourceRetirement{client: client, asset: asset, sources: make(map[string]*trackedLogSource),
		probeChannel: probeEventChannel, probeSystemd: probe}
}

// ObserveBuffered uses the exact source identities that reached the outbox.
func (r *SourceRetirement) ObserveBuffered(ctx context.Context, body []byte, now time.Time,
	publish func(context.Context, []monitoring.DiscoveryEvent) error,
) error {
	if len(body) == 0 {
		clear(r.sources)
		r.resumeAfter = ""
		return nil
	}
	var snapshot struct {
		Discovery struct {
			Candidates []sourceReference
		} `json:"log_source_discovery_v1"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, candidate := range snapshot.Discovery.Candidates {
		if candidate.Fingerprint == "" {
			continue
		}
		seen[candidate.Fingerprint] = true
		r.remember(candidate, now)
	}
	return r.publishAbsent(ctx, seen, now, publish)
}

func isFileCandidate(kind string) bool {
	return kind == "log_file" || kind == "log_glob" || kind == "container_log_glob"
}

type sourceReference struct {
	Fingerprint, Kind, Path, Channel, Unit string
}

func (r *SourceRetirement) remember(candidate sourceReference, now time.Time) {
	previous := r.sources[candidate.Fingerprint]
	if previous == nil && len(r.sources) >= maxTrackedLogSources {
		return
	}
	absent := r.absenceProbe(candidate)
	if absent == nil {
		if previous != nil {
			previous.resetAbsence()
		}
		return
	}
	r.sources[candidate.Fingerprint] = &trackedLogSource{absent: absent, lastSeen: now}
}

func (r *SourceRetirement) absenceProbe(candidate sourceReference) func(context.Context) bool {
	switch {
	case isFileCandidate(candidate.Kind) && filepath.IsAbs(candidate.Path):
		return fileAbsenceProbe(candidate.Path, 0)
	case candidate.Kind == "systemd_unit" && filepath.IsAbs(candidate.Path) &&
		candidate.Unit != "" && filepath.Base(candidate.Path) == candidate.Unit:
		return r.systemdSourceAbsenceProbe(candidate.Path, candidate.Unit, fs.ModeSymlink)
	case candidate.Kind == "journal" && candidate.Path == "/run/systemd/journal/socket":
		return r.systemdSourceAbsenceProbe(candidate.Path, "systemd-journald.service", fs.ModeSocket)
	case candidate.Kind == "eventlog_channel" && validEventChannel(candidate.Channel):
		return func(context.Context) bool {
			presence, err := r.probeChannel(candidate.Channel)
			return err == nil && presence == channelMissing
		}
	default:
		return nil
	}
}

func (r *SourceRetirement) systemdSourceAbsenceProbe(path, unit string, mode fs.FileMode) func(context.Context) bool {
	if r.probeSystemd == nil {
		return nil
	}
	pathAbsent := fileAbsenceProbe(path, mode)
	if pathAbsent == nil {
		return nil
	}
	return func(ctx context.Context) bool {
		if !pathAbsent(ctx) {
			return false
		}
		probeCtx, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
		defer cancel()
		missing, err := r.probeSystemd(probeCtx, unit)
		return err == nil && missing && pathAbsent(ctx)
	}
}

func fileAbsenceProbe(path string, allowedType fs.FileMode) func(context.Context) bool {
	dir := filepath.Dir(path)
	file, fileErr := os.Lstat(path)
	parent, dirErr := os.Stat(dir)
	if fileErr != nil || dirErr != nil || !parent.IsDir() {
		return nil
	}
	fileType := file.Mode().Type()
	if fileType != allowedType && (allowedType != fs.ModeSymlink || fileType != 0) {
		return nil
	}
	return func(context.Context) bool {
		before, beforeErr := os.Stat(dir)
		_, fileErr := os.Lstat(path)
		after, afterErr := os.Stat(dir)
		return beforeErr == nil && afterErr == nil && os.SameFile(parent, before) &&
			os.SameFile(before, after) && before.ModTime().Equal(after.ModTime()) && errors.Is(fileErr, fs.ErrNotExist)
	}
}

func (r *SourceRetirement) publishAbsent(ctx context.Context, seen map[string]bool, now time.Time,
	publish func(context.Context, []monitoring.DiscoveryEvent) error,
) error {
	var fingerprints []string
	var events []monitoring.DiscoveryEvent
	probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	for _, fingerprint := range r.probeOrder() {
		if probeCtx.Err() != nil {
			break
		}
		r.resumeAfter = fingerprint
		if seen[fingerprint] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		source := r.sources[fingerprint]
		if !source.confirmAbsence(probeCtx, now) {
			if now.Sub(source.lastSeen) > 24*time.Hour {
				delete(r.sources, fingerprint)
			}
			continue
		}
		if source.event.ObservedMS == 0 || now.UnixMilli()-source.event.ObservedMS >= retirementFreshness*1000 {
			event, err := monitoring.NewDiscoveryEvent(r.client, r.asset, "log_sources", []string{fingerprint},
				"tombstone", 100, retirementFreshness, now)
			if err != nil {
				return err
			}
			source.event = event
		}
		fingerprints = append(fingerprints, fingerprint)
		events = append(events, source.event)
		if len(events) == maxRetirements {
			break
		}
	}
	if len(events) == 0 {
		return nil
	}
	if err := publish(ctx, events); err != nil {
		return err
	}
	for _, fingerprint := range fingerprints {
		delete(r.sources, fingerprint)
	}
	return nil
}

func (r *SourceRetirement) probeOrder() []string {
	sorted := slices.Sorted(maps.Keys(r.sources))
	position, exact := slices.BinarySearch(sorted, r.resumeAfter)
	if exact {
		position++
	}
	return append(slices.Clone(sorted[position:]), sorted[:position]...)
}

func (s *trackedLogSource) resetAbsence() {
	s.absentSince = time.Time{}
	s.event = monitoring.DiscoveryEvent{}
}

func (s *trackedLogSource) confirmAbsence(ctx context.Context, now time.Time) bool {
	if !s.absent(ctx) {
		s.resetAbsence()
		return false
	}
	if s.absentSince.IsZero() || now.Before(s.absentSince) {
		s.resetAbsence()
		s.absentSince = now
		return false
	}
	return now.Sub(s.absentSince) >= retirementGrace
}
