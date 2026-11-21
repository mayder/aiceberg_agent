package monitoring

import (
	"errors"
	"fmt"
)

// SNMPCompatibilityDefinition describes only the two existing percent series.
// Revision 1 is the adapter contract, not a template/configuration revision.
// History describes the existing 24h observation store; this never changes retention.
func SNMPCompatibilityDefinition(scope Scope, freshnessSec int64) Definition {
	d := Definition{SchemaVersion: 1, Revision: 1, ClientID: scope.ClientID, AssetID: scope.AssetID}
	for _, metric := range []string{"cpu_percent", "memory_percent"} {
		d.Items = append(d.Items, ItemDefinition{
			Key:     fmt.Sprintf("v1:c:%d:a:%d:check:%d:snmp:%s", scope.ClientID, scope.AssetID, scope.CheckID, metric),
			CheckID: scope.CheckID, Metric: metric, Source: "snmp", Kind: "collected", ValueType: "number", Unit: "percent",
			FreshnessSec: freshnessSec, HistoryDays: 1, TrendsDays: 0, Enabled: true, DependsOn: []string{},
		})
	}
	return d
}

// ProjectDefinedSNMP executes this bounded compatibility adapter after validating
// the definition against caller-resolved authority. It never probes or writes.
func ProjectDefinedSNMP(d Definition, authority DefinitionAuthority, observation Observation, observedMS, nowMS int64) (Batch, error) {
	if err := ValidateDefinition(d, authority); err != nil {
		return Batch{}, err
	}
	if len(d.Items) != 2 || len(authority.CheckIDs) != 1 {
		return Batch{}, errors.New("unsupported_monitoring_definition")
	}
	freshness := d.Items[0].FreshnessSec
	seen := map[string]bool{}
	for _, item := range d.Items {
		if item.CheckID != authority.CheckIDs[0] || item.Source != "snmp" || item.Kind != "collected" ||
			item.ValueType != "number" || item.Unit != "percent" || !item.Enabled || item.FreshnessSec != freshness ||
			(item.Metric != "cpu_percent" && item.Metric != "memory_percent") || seen[item.Metric] {
			return Batch{}, errors.New("unsupported_monitoring_definition")
		}
		seen[item.Metric] = true
	}
	return ProjectSNMP(Scope{ClientID: authority.ClientID, AssetID: authority.AssetID, CheckID: authority.CheckIDs[0]}, observation, observedMS, nowMS, freshness)
}
