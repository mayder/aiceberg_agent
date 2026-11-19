// Package monitoring defines the bounded compatibility contract for monitoring samples.
package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

type Scope struct {
	ClientID int64 `json:"client_id"`
	AssetID  int64 `json:"asset_id"`
	CheckID  int64 `json:"check_id"`
}

type Observation struct {
	Status  string         `json:"status"`
	Partial bool           `json:"partial"`
	Final   bool           `json:"final"`
	Payload map[string]any `json:"payload"`
}

type Sample struct {
	ItemKey string   `json:"item_key"`
	Metric  string   `json:"metric"`
	Unit    string   `json:"unit"`
	State   string   `json:"state"`
	Gap     string   `json:"gap"`
	Value   *float64 `json:"value"`
}

type Batch struct {
	SchemaVersion int      `json:"schema_version"`
	Source        string   `json:"source"`
	Scope         Scope    `json:"scope"`
	ObservedAtMS  int64    `json:"observed_at_ms"`
	FreshnessSec  int64    `json:"freshness_sec"`
	Samples       []Sample `json:"samples"`
}

// ProjectSNMP consumes an existing observation, without collection, writes or alarm effects.
// Scope must come from an authorized job/check, never from untrusted metric payloads.
func ProjectSNMP(scope Scope, observation Observation, observedMS, nowMS, freshnessSec int64) (Batch, error) {
	for _, id := range []int64{scope.ClientID, scope.AssetID, scope.CheckID} {
		if id < 1 || id > 9007199254740991 {
			return Batch{}, errors.New("invalid_monitoring_scope")
		}
	}
	if observedMS < 1 || nowMS < 1 || observedMS > 253402300799999 || nowMS > 253402300799999 || freshnessSec < 1 || freshnessSec > 604800 {
		return Batch{}, errors.New("invalid_monitoring_clock")
	}
	batch := Batch{SchemaVersion: 1, Source: "agentless_snmp", Scope: scope, ObservedAtMS: observedMS, FreshnessSec: freshnessSec, Samples: make([]Sample, 0, 2)}
	gap := observationGap(observation, observedMS, nowMS, freshnessSec)
	nested, _ := observation.Payload["cpu_memory"].(map[string]any)
	for _, metric := range []string{"cpu_percent", "memory_percent"} {
		raw := observation.Payload[metric]
		if raw == nil {
			raw = nested[metric]
		}
		sample := projectPercent(scope, metric, raw, gap)
		batch.Samples = append(batch.Samples, sample)
	}
	return batch, nil
}

func projectPercent(scope Scope, metric string, raw any, gap string) Sample {
	value, valid := percent(raw)
	if gap == "" && raw == nil {
		gap = "missing"
	} else if gap == "" && !valid {
		gap = "invalid"
	}
	sample := Sample{ItemKey: fmt.Sprintf("v1:c:%d:a:%d:check:%d:snmp:%s", scope.ClientID, scope.AssetID, scope.CheckID, metric), Metric: metric, Unit: "percent", State: "unknown", Gap: gap}
	if gap == "" {
		sample.State, sample.Value = "ok", &value
	}
	return sample
}

func observationGap(observation Observation, observed, now, freshness int64) string {
	if observation.Status != "ok" {
		return "observation_failed"
	}
	if observation.Partial || !observation.Final || legacyFlag(observation.Payload["is_partial"]) {
		return "partial"
	}
	if legacyFlag(observation.Payload["time_budget_exceeded"]) {
		return "time_budget"
	}
	if observed > now {
		return "future"
	}
	if now-observed > freshness*1000 {
		return "stale"
	}
	return ""
}

// Legacy flags follow PHP empty(), preserving conservative handling of malformed flags.
func legacyFlag(raw any) bool {
	switch v := raw.(type) {
	case nil:
		return false
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != "" && v != "0"
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

var decimalNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func percent(raw any) (float64, bool) {
	var number float64
	switch value := raw.(type) {
	case float64:
		number = value
	case float32:
		number = float64(value)
	case int:
		number = float64(value)
	case int64:
		number = float64(value)
	case json.Number:
		return percent(string(value))
	case string:
		text := strings.Trim(value, " \t\n\r\v\f")
		if !decimalNumber.MatchString(text) {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0) && number >= 0 && number <= 100
}
