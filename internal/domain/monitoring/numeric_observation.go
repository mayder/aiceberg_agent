package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// NumericSelector identifies a specific configured remote value.
type NumericSelector struct {
	ID          string
	Fingerprint string
	Unit        string
}

func (s NumericSelector) Reference() string { return s.ID + ":" + s.Fingerprint }

func NumericProbeValues(payload map[string]any, kind string) map[string]any {
	values, _ := payload[kind+"_values_v1"].(map[string]any)
	return values
}

func IsNumericProbe(source string) bool {
	return source == OpenMetricsSource || source == JolokiaSource || source == PostgresSource || source == IPMISource || source == VCenterSource || source == SSHSource || source == MySQLSource || source == TelnetSource || source == BrowserSource || source == JMXSource
}

type NumericProbeProjection func(Scope, Observation, int64, int64) (Batch, error)

func ProjectConfiguredNumericProbe(scope Scope, source string, config map[string]any, observation Observation, observed, freshness int64) (Batch, error) {
	project, err := PrepareConfiguredNumericProbe(source, config)
	if err != nil {
		return Batch{}, err
	}
	return project(scope, observation, observed, freshness)
}

// PrepareConfiguredNumericProbe validates the selectors once for all observations in a window.
func PrepareConfiguredNumericProbe(source string, config map[string]any) (NumericProbeProjection, error) {
	switch source {
	case OpenMetricsSource:
		return prepareNumericProbe(config, OpenMetricsSelectors, ProjectOpenMetrics)
	case JolokiaSource:
		return prepareNumericProbe(config, JolokiaSelectors, ProjectJolokia)
	case PostgresSource:
		return prepareNumericProbe(config, PostgresSelectors, ProjectPostgres)
	case IPMISource:
		return prepareNumericProbe(config, IPMISelectors, ProjectIPMI)
	case JMXSource:
		return prepareNumericProbe(config, JMXSelectors, ProjectJMX)
	case BrowserSource:
		return prepareNumericProbe(config, BrowserSelectors, ProjectBrowser)
	case TelnetSource:
		return prepareNumericProbe(config, TelnetSelectors, ProjectTelnet)
	case MySQLSource:
		return prepareNumericProbe(config, MySQLSelectors, ProjectMySQL)
	case SSHSource:
		return prepareNumericProbe(config, SSHSelectors, ProjectSSH)
	case VCenterSource:
		return prepareNumericProbe(config, VCenterSelectors, ProjectVCenter)
	default:
		return nil, errors.New("invalid_numeric_probe_source")
	}
}

func prepareNumericProbe[T any](config map[string]any, parse func(map[string]any) (T, error),
	project func(Scope, T, Observation, int64, int64) (Batch, error),
) (NumericProbeProjection, error) {
	selector, err := parse(config)
	if err != nil {
		return nil, err
	}
	return func(scope Scope, observation Observation, observed, freshness int64) (Batch, error) {
		return project(scope, selector, observation, observed, freshness)
	}, nil
}

func NumericProbeValue(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	case json.Number:
		parsed, err := value.Float64()
		return parsed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return 0, false
	}
}

func projectNumericProbe(scope Scope, source string, selectors []NumericSelector, observation Observation, observed, freshness int64) (Batch, error) {
	for _, id := range []int64{scope.ClientID, scope.AssetID, scope.CheckID} {
		if id < 1 || id > 9007199254740991 {
			return Batch{}, errors.New("invalid_monitoring_scope")
		}
	}
	if observed < 1 || observed > 253402300799999 || freshness < 60 || freshness > 604800 {
		return Batch{}, errors.New("invalid_monitoring_clock")
	}
	values := NumericProbeValues(observation.Payload, source)
	gap := observationGap(observation, observed, observed, freshness)
	samples := make([]Sample, 0, len(selectors))
	for _, selector := range selectors {
		metric := selector.ID + "." + selector.Fingerprint[:24]
		key := fmt.Sprintf("v1:c:%d:a:%d:check:%d:%s:%s", scope.ClientID, scope.AssetID, scope.CheckID, source, metric)
		unit := selector.Unit
		if unit == "" {
			unit = "none"
		}
		sample := Sample{ItemKey: key, Metric: metric, Unit: unit, State: "unknown", Gap: gap}
		if gap == "" {
			sample.Gap = "missing"
			if value, ok := NumericProbeValue(values[selector.Reference()]); ok {
				sample.State, sample.Gap, sample.Value = "ok", "", &value
			}
		}
		samples = append(samples, sample)
	}
	return Batch{SchemaVersion: 1, Source: "agentless_" + source, Scope: scope,
		ObservedAtMS: observed, FreshnessSec: freshness, Samples: samples}, nil
}
