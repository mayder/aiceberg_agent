package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"regexp"
)

const BrowserSource = "browser"

var (
	browserMetricID   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,38}$`)
	browserRecipeHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
	errBrowserConfig  = errors.New("invalid_browser_selection")
)

type BrowserSelection struct {
	Digest  string
	Metrics []BrowserMetric
}

type BrowserMetric struct {
	ID     string
	Preset string
}

func BrowserSelectors(config map[string]any) (BrowserSelection, error) {
	digest, _ := config["recipe_sha256"].(string)
	metrics, ok := config["metrics"].([]any)
	if len(config) != 2 || !browserRecipeHash.MatchString(digest) || !ok || len(metrics) < 1 || len(metrics) > 2 {
		return BrowserSelection{}, errBrowserConfig
	}
	result := BrowserSelection{Digest: digest, Metrics: make([]BrowserMetric, 0, len(metrics))}
	ids := make(map[string]bool, len(metrics))
	for _, value := range metrics {
		metric, _ := value.(map[string]any)
		id, _ := metric["id"].(string)
		preset, _ := metric["preset"].(string)
		if len(metric) != 2 || !browserMetricID.MatchString(id) || ids[id] || browserMetricUnit(preset) == "" {
			return BrowserSelection{}, errBrowserConfig
		}
		ids[id] = true
		result.Metrics = append(result.Metrics, BrowserMetric{ID: id, Preset: preset})
	}
	return result, nil
}

func browserMetricUnit(preset string) string {
	switch preset {
	case "duration_ms":
		return "milliseconds"
	case "steps_completed":
		return "none"
	default:
		return ""
	}
}

func validBrowserValue(preset string, value float64) bool {
	if value != math.Trunc(value) {
		return false
	}
	switch preset {
	case "duration_ms":
		return value >= 0 && value <= 35000
	case "steps_completed":
		return value >= 3 && value <= 10
	default:
		return false
	}
}

func (s BrowserSelection) selector(metric BrowserMetric) NumericSelector {
	fingerprint := sha256.Sum256([]byte(s.Digest + "\x00" + metric.Preset))
	return NumericSelector{ID: metric.ID, Fingerprint: hex.EncodeToString(fingerprint[:]), Unit: browserMetricUnit(metric.Preset)}
}

func (s BrowserSelection) Reference(metric BrowserMetric) string {
	return s.selector(metric).Reference()
}

func ProjectBrowser(scope Scope, selection BrowserSelection, observation Observation, observed, freshness int64) (Batch, error) {
	series := make([]NumericSelector, 0, len(selection.Metrics))
	values := make(map[string]any, len(selection.Metrics))
	var raw map[string]any
	if digest, _ := observation.Payload["browser_recipe_sha256"].(string); digest == selection.Digest {
		raw = NumericProbeValues(observation.Payload, BrowserSource)
	}
	for _, metric := range selection.Metrics {
		selector := selection.selector(metric)
		series = append(series, selector)
		if value, ok := NumericProbeValue(raw[selector.Reference()]); ok && validBrowserValue(metric.Preset, value) {
			values[selector.Reference()] = value
		}
	}
	observation.Payload = map[string]any{"browser_values_v1": values}
	return projectNumericProbe(scope, BrowserSource, series, observation, observed, freshness)
}
