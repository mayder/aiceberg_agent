package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// TCPCompatibilityDefinition describes the elapsed milliseconds of the existing
// connection attempt. It never grants authority or changes observation retention.
func TCPCompatibilityDefinition(scope Scope, freshnessSec int64) Definition {
	return Definition{SchemaVersion: 1, Revision: 1, ClientID: scope.ClientID, AssetID: scope.AssetID, Items: []ItemDefinition{{
		Key:     fmt.Sprintf("v1:c:%d:a:%d:check:%d:tcp:latency_ms", scope.ClientID, scope.AssetID, scope.CheckID),
		CheckID: scope.CheckID, Metric: "latency_ms", Source: "tcp", Kind: "collected", ValueType: "integer", Unit: "milliseconds",
		FreshnessSec: freshnessSec, HistoryDays: 1, TrendsDays: 0, Enabled: true, DependsOn: []string{},
	}}}
}

// ProjectDefinedTCP ignores payload-proposed metrics. latencyMS is the dedicated
// legacy observation field, resolved by the caller, including a genuine zero.
func ProjectDefinedTCP(d Definition, authority DefinitionAuthority, observation Observation, latencyMS any, observedMS, nowMS int64) (Batch, error) {
	if err := ValidateDefinition(d, authority); err != nil {
		return Batch{}, err
	}
	if len(d.Items) != 1 || len(authority.CheckIDs) != 1 {
		return Batch{}, errors.New("unsupported_monitoring_definition")
	}
	item := d.Items[0]
	if item.CheckID != authority.CheckIDs[0] || item.Source != "tcp" || item.Metric != "latency_ms" || item.Kind != "collected" || item.ValueType != "integer" || item.Unit != "milliseconds" || !item.Enabled {
		return Batch{}, errors.New("unsupported_monitoring_definition")
	}
	if observedMS < 1 || nowMS < 1 || observedMS > 253402300799999 || nowMS > 253402300799999 {
		return Batch{}, errors.New("invalid_monitoring_clock")
	}
	gap := observationGap(observation, observedMS, nowMS, item.FreshnessSec)
	value, valid := tcpMilliseconds(latencyMS)
	if gap == "" && latencyMS == nil {
		gap = "missing"
	} else if gap == "" && !valid {
		gap = "invalid"
	}
	sample := Sample{ItemKey: item.Key, Metric: item.Metric, Unit: item.Unit, State: "unknown", Gap: gap}
	if gap == "" {
		sample.State = "ok"
		sample.Value = &value
	}
	return Batch{SchemaVersion: 1, Source: "agentless_tcp", Scope: Scope{ClientID: authority.ClientID, AssetID: authority.AssetID, CheckID: item.CheckID}, ObservedAtMS: observedMS, FreshnessSec: item.FreshnessSec, Samples: []Sample{sample}}, nil
}

func tcpMilliseconds(raw any) (float64, bool) {
	var n float64
	switch v := raw.(type) {
	case int:
		n = float64(v)
	case int64:
		n = float64(v)
	case float64:
		n = v
	case json.Number:
		return tcpMilliseconds(string(v))
	case string:
		s := strings.Trim(v, " \t\n\r\v\f")
		if !decimalNumber.MatchString(s) {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	return n, !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 86400000 && math.Trunc(n) == n
}
