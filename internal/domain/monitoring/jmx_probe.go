package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"regexp"
)

const JMXSource = "jmx"

var (
	jmxProbeReference = regexp.MustCompile(`^jmx_probe_profile:[1-9][0-9]{0,8}$`)
	jmxMetricID       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	errJMXSelection   = errors.New("invalid_jmx_selection")
)

type JMXSelection struct {
	CredentialRef string
	Metrics       []JMXMetric
}

type JMXMetric struct {
	ID     string
	Preset string
}

func ValidJMXReference(reference string) bool {
	return jmxProbeReference.MatchString(reference)
}

func JMXSelectors(config map[string]any) (JMXSelection, error) {
	reference, _ := config["credential_ref"].(string)
	metrics, ok := config["metrics"].([]any)
	if len(config) != 2 || !ValidJMXReference(reference) || !ok || len(metrics) < 1 || len(metrics) > 32 {
		return JMXSelection{}, errJMXSelection
	}
	result := JMXSelection{CredentialRef: reference, Metrics: make([]JMXMetric, 0, len(metrics))}
	seen := make(map[string]bool, len(metrics))
	for _, value := range metrics {
		raw, _ := value.(map[string]any)
		id, _ := raw["id"].(string)
		preset, _ := raw["preset"].(string)
		if len(raw) != 2 || !jmxMetricID.MatchString(id) || seen[id] || jmxProbeUnit(preset) == "" {
			return JMXSelection{}, errJMXSelection
		}
		seen[id] = true
		result.Metrics = append(result.Metrics, JMXMetric{ID: id, Preset: preset})
	}
	return result, nil
}

func jmxProbeUnit(preset string) string {
	switch preset {
	case "live_threads", "daemon_threads", "loaded_classes":
		return "none"
	case "uptime_ms":
		return "milliseconds"
	case "heap_used_bytes", "heap_max_bytes":
		return "bytes"
	default:
		return ""
	}
}

func ValidJMXMetric(preset string) bool { return jmxProbeUnit(preset) != "" }

func (s JMXSelection) selector(metric JMXMetric) NumericSelector {
	hash := sha256.Sum256([]byte(s.CredentialRef + "\x00" + metric.Preset))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(hash[:]), Unit: jmxProbeUnit(metric.Preset)}
}

func (s JMXSelection) Reference(metric JMXMetric) string {
	return s.selector(metric).Reference()
}

func ProjectJMX(scope Scope, selection JMXSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	raw := NumericProbeValues(observation.Payload, JMXSource)
	values := make(map[string]any, len(selection.Metrics))
	for _, metric := range selection.Metrics {
		selector := selection.selector(metric)
		series = append(series, selector)
		reference := selector.Reference()
		if value, ok := NumericProbeValue(raw[reference]); ok && value >= 0 && value <= 9007199254740991 && value == math.Trunc(value) {
			values[reference] = value
		}
	}
	observation.Payload = map[string]any{"jmx_values_v1": values}
	return projectNumericProbe(scope, JMXSource, series, observation, observed, freshness)
}
