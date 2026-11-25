package monitoring

import (
	"errors"
	"fmt"
	"regexp"
)

var definitionMetric = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
var definitionUnit = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// ValidateDefinition requires authority resolved by the caller, never from definition fields.
// A valid definition is not permission to run collectors or change ownership/history.
func ValidateDefinition(d Definition, authority DefinitionAuthority) error {
	checks, err := definitionAuthority(authority)
	if err != nil {
		return err
	}
	if d.SchemaVersion != 1 || !definitionInteger(d.Revision, 1, 9007199254740991) || len(d.Items) < 1 || len(d.Items) > MaxDefinitionItems {
		return errors.New("invalid_monitoring_definition")
	}
	if d.ClientID != authority.ClientID || d.AssetID != authority.AssetID {
		return errors.New("monitoring_definition_scope_mismatch")
	}
	items := make(map[string]ItemDefinition, len(d.Items))
	for _, item := range d.Items {
		if err := validateDefinitionItem(item, authority, checks); err != nil {
			return err
		}
		if _, exists := items[item.Key]; exists {
			return errors.New("duplicate_monitoring_item")
		}
		items[item.Key] = item
	}
	return validateDefinitionGraph(items, d.Items)
}

func definitionAuthority(a DefinitionAuthority) (map[int64]bool, error) {
	if !definitionInteger(a.ClientID, 1, 9007199254740991) || !definitionInteger(a.AssetID, 1, 9007199254740991) || len(a.CheckIDs) < 1 || len(a.CheckIDs) > MaxDefinitionItems {
		return nil, errors.New("invalid_monitoring_authority")
	}
	ids := make(map[int64]bool, len(a.CheckIDs))
	for _, id := range a.CheckIDs {
		if !definitionInteger(id, 1, 9007199254740991) || ids[id] {
			return nil, errors.New("invalid_monitoring_authority")
		}
		ids[id] = true
	}
	return ids, nil
}

func validateDefinitionItem(i ItemDefinition, a DefinitionAuthority, checks map[int64]bool) error {
	if !checks[i.CheckID] {
		return errors.New("monitoring_definition_scope_mismatch")
	}
	if !definitionMetric.MatchString(i.Metric) || !definitionUnit.MatchString(i.Unit) ||
		!definitionEnum(i.Source, "snmp", "icmp", "tcp", "http", "dns", "tls", "openmetrics", "jolokia", "postgresql", IPMISource, VCenterSource, SSHSource, MySQLSource, TelnetSource, BrowserSource, JMXSource, "snmp_trap", "host", "custom", "derived") ||
		!definitionEnum(i.Kind, "collected", "dependent", "calculated") || !definitionEnum(i.ValueType, "number", "integer", "boolean", "text", "event") ||
		!definitionInteger(i.FreshnessSec, 1, 604800) || !definitionInteger(i.HistoryDays, 1, 3650) || !definitionInteger(i.TrendsDays, 0, 3650) || i.DependsOn == nil || len(i.DependsOn) > 16 {
		return errors.New("invalid_monitoring_item")
	}
	if i.Source == "snmp_trap" && (i.Metric != "notification" || i.ValueType != "event" || i.Unit != "none") {
		return errors.New("invalid_monitoring_item")
	}
	key := fmt.Sprintf("v1:c:%d:a:%d:check:%d:%s:%s", a.ClientID, a.AssetID, i.CheckID, i.Source, i.Metric)
	if i.Key != key {
		return errors.New("invalid_monitoring_item")
	}
	numeric := definitionEnum(i.ValueType, "number", "integer")
	if i.TrendsDays != 0 && (!numeric || i.TrendsDays < i.HistoryDays) {
		return errors.New("invalid_monitoring_item")
	}
	n := len(i.DependsOn)
	if (i.Kind == "collected" && (n != 0 || i.Source == "derived")) || (i.Kind != "collected" && (n == 0 || i.Source != "derived")) || (i.Kind == "dependent" && n != 1) || (i.Kind == "calculated" && !numeric) {
		return errors.New("invalid_monitoring_item")
	}
	seen := make(map[string]bool, n)
	for _, key := range i.DependsOn {
		if len(key) > 192 || seen[key] {
			return errors.New("invalid_monitoring_item")
		}
		seen[key] = true
	}
	return nil
}

func validateDefinitionGraph(items map[string]ItemDefinition, ordered []ItemDefinition) error {
	for _, item := range ordered {
		for _, parent := range item.DependsOn {
			master, ok := items[parent]
			if !ok {
				return errors.New("monitoring_dependency_missing")
			}
			if item.Enabled && !master.Enabled {
				return errors.New("monitoring_dependency_disabled")
			}
		}
	}
	states := make(map[string]int, len(items))
	depths := make(map[string]int, len(items))
	for _, item := range ordered {
		if _, err := definitionDepth(item.Key, items, states, depths); err != nil {
			return err
		}
	}
	return nil
}

func definitionDepth(key string, items map[string]ItemDefinition, states, depths map[string]int) (int, error) {
	if states[key] == 1 {
		return 0, errors.New("monitoring_dependency_cycle")
	}
	if states[key] == 2 {
		return depths[key], nil
	}
	states[key] = 1
	depth := 1
	for _, parent := range items[key].DependsOn {
		n, err := definitionDepth(parent, items, states, depths)
		if err != nil {
			return 0, err
		}
		if n+1 > depth {
			depth = n + 1
		}
	}
	if depth > MaxDefinitionDepth {
		return 0, errors.New("monitoring_dependency_depth")
	}
	states[key] = 2
	depths[key] = depth
	return depth, nil
}

func definitionInteger(v, min, max int64) bool { return v >= min && v <= max }
func definitionEnum(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
