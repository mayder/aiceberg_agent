package configfailure

import (
	"errors"
	"math"
	"regexp"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s State) Validate() error {
	if s.Sequence == math.MaxUint64 || s.SchemaVersion != SchemaVersion || len(s.Entries) > MaxEntries || len(s.Pending) > MaxPending || s.Dropped < 0 || s.Dropped > MaxCount {
		return errors.New("invalid diagnostic state")
	}
	seen := make(map[string]bool)
	for _, entry := range s.Entries {
		if seen[entry.Diagnostic.Fingerprint] || entry.LastQueued < 0 || entry.ResolvedAt < 0 {
			return errors.New("invalid diagnostic history")
		}
		seen[entry.Diagnostic.Fingerprint] = true
		if len(entry.Version) > 80 || !digestPattern.MatchString(entry.ConfigHash) || !digestPattern.MatchString(entry.IdentityDigest) || !validFacts(entry.Diagnostic, false) {
			return errors.New("invalid diagnostic condition")
		}
	}
	if s.Active != "" {
		if _, ok := s.Current(); !ok {
			return errors.New("invalid active diagnostic")
		}
	}
	seen = make(map[string]bool)
	for _, delivery := range s.Pending {
		if seen[delivery.Diagnostic.ReportID] {
			return errors.New("duplicate diagnostic receipt")
		}
		seen[delivery.Diagnostic.ReportID] = true
		if len(delivery.Version) > 80 || !digestPattern.MatchString(delivery.ConfigHash) || !validFacts(delivery.Diagnostic, true) {
			return errors.New("invalid pending diagnostic")
		}
	}
	return nil
}

func validFacts(r Report, delivery bool) bool {
	if r.SchemaVersion != SchemaVersion || !digestPattern.MatchString(r.Fingerprint) || r.FirstSeen < 0 || r.LastSeen < r.FirstSeen || r.NextRetry < 0 || r.LastSeen > 253402300799000 || r.NextRetry > 253402300799000 || r.Count < 1 || r.Count > MaxCount || r.Attempt < 1 || r.Attempt > MaxCount {
		return false
	}
	kinds := map[string]string{"path_conflict": "persistent", "permission_denied": "persistent", "disk_full": "persistent", "read_only": "persistent", "concurrent_change": "transient", "io_temporary": "transient", "unknown": "unknown"}
	if kind, ok := kinds[r.Code]; !ok || kind != r.Kind {
		return false
	}
	if !delivery {
		return true
	}
	if !digestPattern.MatchString(r.ReportID) {
		return false
	}
	switch r.Phase {
	case "first", "changed", "summary":
		return true
	case "recovered":
		return r.NextRetry == 0
	default:
		return false
	}
}
