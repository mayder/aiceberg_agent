package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

const PostgresSource = "postgresql"

var (
	postgresRef          = regexp.MustCompile(`^postgres_profile:[1-9][0-9]{0,8}$`)
	postgresIdentity     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,62}$`)
	postgresMetricID     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errPostgresSelection = errors.New("invalid_postgresql_selection")
)

type PostgresSelection struct {
	CredentialRef string
	Database      string
	Metrics       []PostgresMetric
}

type PostgresMetric struct {
	ID     string
	Preset string
}

func ValidPostgresReference(ref string) bool { return postgresRef.MatchString(ref) }
func ValidPostgresIdentity(name string) bool { return postgresIdentity.MatchString(name) }

func PostgresSelectors(config map[string]any) (PostgresSelection, error) {
	ref, _ := config["credential_ref"].(string)
	db, _ := config["database"].(string)
	values, ok := config["metrics"].([]any)
	if len(config) != 3 || !ValidPostgresReference(ref) || !ValidPostgresIdentity(db) || !ok || len(values) < 1 || len(values) > 32 {
		return PostgresSelection{}, errPostgresSelection
	}
	result := PostgresSelection{CredentialRef: ref, Database: db, Metrics: make([]PostgresMetric, 0, len(values))}
	seen := make(map[string]bool, len(values))
	for _, raw := range values {
		metric, _ := raw.(map[string]any)
		id, _ := metric["id"].(string)
		preset, _ := metric["preset"].(string)
		if len(metric) != 2 || !postgresMetricID.MatchString(id) || !validPostgresPreset(preset) || seen[id] {
			return PostgresSelection{}, errPostgresSelection
		}
		seen[id] = true
		result.Metrics = append(result.Metrics, PostgresMetric{ID: id, Preset: preset})
	}
	return result, nil
}

func validPostgresPreset(candidate string) bool {
	switch candidate {
	case "database_connections", "database_commits", "database_rollbacks", "server_uptime_seconds":
		return true
	default:
		return false
	}
}

func (s PostgresSelection) numericSelector(metric PostgresMetric) NumericSelector {
	hash := sha256.Sum256([]byte(strings.Join([]string{s.CredentialRef, s.Database, metric.Preset}, "\x00")))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(hash[:])}
}

func (s PostgresSelection) Reference(metric PostgresMetric) string {
	return s.numericSelector(metric).Reference()
}

func ProjectPostgres(scope Scope, selection PostgresSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		series = append(series, selection.numericSelector(metric))
	}
	return projectNumericProbe(scope, PostgresSource, series, observation, observed, freshness)
}
