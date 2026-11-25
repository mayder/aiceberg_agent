package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
)

const MySQLSource = "mysql"

var (
	mysqlProbeReference = regexp.MustCompile(`^mysql_probe_profile:[1-9][0-9]{0,8}$`)
	mysqlMetricID       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errMySQLSelection   = errors.New("invalid_mysql_selection")
)

type MySQLSelection struct {
	CredentialRef string
	Metrics       []MySQLMetric
}

type MySQLMetric struct {
	ID     string
	Preset string
}

func ValidMySQLReference(reference string) bool {
	return mysqlProbeReference.MatchString(reference)
}

func MySQLSelectors(config map[string]any) (MySQLSelection, error) {
	reference, _ := config["credential_ref"].(string)
	metrics, ok := config["metrics"].([]any)
	if len(config) != 2 || !ValidMySQLReference(reference) || !ok || len(metrics) < 1 || len(metrics) > 32 {
		return MySQLSelection{}, errMySQLSelection
	}
	result := MySQLSelection{CredentialRef: reference, Metrics: make([]MySQLMetric, 0, len(metrics))}
	seen := make(map[string]bool, len(metrics))
	for _, value := range metrics {
		raw, _ := value.(map[string]any)
		id, _ := raw["id"].(string)
		preset, _ := raw["preset"].(string)
		if len(raw) != 2 || !mysqlMetricID.MatchString(id) || seen[id] || mysqlProbeUnit(preset) == "" {
			return MySQLSelection{}, errMySQLSelection
		}
		seen[id] = true
		result.Metrics = append(result.Metrics, MySQLMetric{ID: id, Preset: preset})
	}
	return result, nil
}

func mysqlProbeUnit(preset string) string {
	switch preset {
	case "server_connections", "server_questions", "server_aborted_connects":
		return "none"
	case "server_uptime_seconds":
		return "seconds"
	default:
		return ""
	}
}

func (s MySQLSelection) selector(metric MySQLMetric) NumericSelector {
	hash := sha256.Sum256([]byte(s.CredentialRef + "\x00" + metric.Preset))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(hash[:]), Unit: mysqlProbeUnit(metric.Preset)}
}

func (s MySQLSelection) Reference(metric MySQLMetric) string {
	return s.selector(metric).Reference()
}

func ProjectMySQL(scope Scope, selection MySQLSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		series = append(series, selection.selector(metric))
	}
	return projectNumericProbe(scope, MySQLSource, series, observation, observed, freshness)
}
