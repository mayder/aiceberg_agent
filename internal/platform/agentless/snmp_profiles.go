package agentless

import (
	"strings"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type snmpFetchMode string

const (
	snmpFetchAuto     snmpFetchMode = "auto"
	snmpFetchGetOnly  snmpFetchMode = "get_only"
	snmpFetchWalkOnly snmpFetchMode = "walk_only"
)

const (
	defaultSNMPCollectionProfile = "switch_noc"
	defaultSNMPMaxRows           = 200
	defaultSNMPTimeBudgetMs      = 15000
)

const (
	snmpCollectionLegacy         = "legacy"
	snmpCollectionMetricsFast    = "metrics_fast"
	snmpCollectionSwitchportSlow = "switchport_slow"
	snmpCollectionDeepDiag       = "deep_diag"
)

type snmpGroupDef struct {
	Scalars []string
	Tables  []string
}

type snmpSegmentDefault struct {
	Groups       []string
	FetchMode    snmpFetchMode
	MaxRows      int
	TimeBudgetMs int
}

type snmpPlan struct {
	Host              string
	ProfileID         int
	CollectionProfile string
	CollectionKind    string
	FetchMode         snmpFetchMode
	MaxRows           int
	TimeBudgetMs      int
	Groups            []string
	CustomGet         []string
	CustomWalk        []string
}

var snmpGroupDefs = map[string]snmpGroupDef{
	"system": {
		Scalars: []string{
			"1.3.6.1.2.1.1.1.0",
			"1.3.6.1.2.1.1.2.0",
			"1.3.6.1.2.1.1.3.0",
			"1.3.6.1.2.1.1.4.0",
			"1.3.6.1.2.1.1.5.0",
			"1.3.6.1.2.1.1.6.0",
			"1.3.6.1.2.1.1.7.0",
		},
	},
	"interfaces_status": {
		Scalars: []string{
			"1.3.6.1.2.1.2.1.0",
		},
		Tables: []string{
			"1.3.6.1.2.1.2.2.1.2",
			"1.3.6.1.2.1.2.2.1.3",
			"1.3.6.1.2.1.2.2.1.4",
			"1.3.6.1.2.1.2.2.1.5",
			"1.3.6.1.2.1.2.2.1.6",
			"1.3.6.1.2.1.2.2.1.7",
			"1.3.6.1.2.1.2.2.1.8",
			"1.3.6.1.2.1.2.2.1.9",
			"1.3.6.1.2.1.31.1.1.1.1",
			"1.3.6.1.2.1.31.1.1.1.18",
		},
	},
	"interfaces_counters": {
		Tables: []string{
			"1.3.6.1.2.1.2.2.1.10",
			"1.3.6.1.2.1.2.2.1.11",
			"1.3.6.1.2.1.2.2.1.13",
			"1.3.6.1.2.1.2.2.1.14",
			"1.3.6.1.2.1.2.2.1.16",
			"1.3.6.1.2.1.2.2.1.17",
			"1.3.6.1.2.1.2.2.1.19",
			"1.3.6.1.2.1.2.2.1.20",
			"1.3.6.1.2.1.31.1.1.1.6",
			"1.3.6.1.2.1.31.1.1.1.10",
		},
	},
	"ip_stats": {
		Scalars: []string{
			"1.3.6.1.2.1.4.1.0",
			"1.3.6.1.2.1.4.3.0",
			"1.3.6.1.2.1.4.9.0",
			"1.3.6.1.2.1.4.10.0",
			"1.3.6.1.2.1.4.11.0",
			"1.3.6.1.2.1.4.12.0",
		},
	},
	"bridge_fdb": {
		Tables: []string{
			"1.3.6.1.2.1.17.4.3.1.1",
			"1.3.6.1.2.1.17.4.3.1.2",
			"1.3.6.1.2.1.17.4.3.1.3",
		},
	},
	"vlan": {
		Tables: []string{
			"1.3.6.1.2.1.17.1.4.1.2",
			"1.3.6.1.2.1.17.7.1.4.2.1.4",
			"1.3.6.1.2.1.17.7.1.4.2.1.5",
			"1.3.6.1.2.1.17.7.1.4.5.1.1",
			"1.3.6.1.2.1.17.7.1.4.3.1.1",
		},
	},
	"stp": {
		Scalars: []string{
			"1.3.6.1.2.1.17.2.2.0",
			"1.3.6.1.2.1.17.2.5.0",
			"1.3.6.1.2.1.17.2.6.0",
			"1.3.6.1.2.1.17.2.7.0",
		},
		Tables: []string{
			"1.3.6.1.2.1.17.2.15.1.3",
			"1.3.6.1.2.1.17.2.15.1.4",
		},
	},
	"lldp_local": {
		Scalars: []string{
			"1.0.8802.1.1.2.1.3.2.0",
			"1.0.8802.1.1.2.1.3.3.0",
			"1.0.8802.1.1.2.1.3.4.0",
		},
		Tables: []string{
			"1.0.8802.1.1.2.1.3.7.1.2",
			"1.0.8802.1.1.2.1.3.7.1.3",
			"1.0.8802.1.1.2.1.3.7.1.4",
		},
	},
	"lldp_remote": {
		Tables: []string{
			"1.0.8802.1.1.2.1.4.1.1.4",
			"1.0.8802.1.1.2.1.4.1.1.5",
			"1.0.8802.1.1.2.1.4.1.1.6",
			"1.0.8802.1.1.2.1.4.1.1.7",
			"1.0.8802.1.1.2.1.4.1.1.8",
			"1.0.8802.1.1.2.1.4.1.1.9",
			"1.0.8802.1.1.2.1.4.1.1.10",
			"1.0.8802.1.1.2.1.4.1.1.12",
		},
	},
	"entity": {
		Tables: []string{
			"1.3.6.1.2.1.47.1.1.1.1.2",
			"1.3.6.1.2.1.47.1.1.1.1.3",
			"1.3.6.1.2.1.47.1.1.1.1.5",
			"1.3.6.1.2.1.47.1.1.1.1.7",
			"1.3.6.1.2.1.47.1.1.1.1.10",
			"1.3.6.1.2.1.47.1.1.1.1.11",
			"1.3.6.1.2.1.47.1.1.1.1.12",
			"1.3.6.1.2.1.47.1.1.1.1.13",
			"1.3.6.1.2.1.47.1.1.1.1.14",
		},
	},
	"sensors": {
		Tables: []string{
			"1.3.6.1.2.1.99.1.1.1.1",
			"1.3.6.1.2.1.99.1.1.1.2",
			"1.3.6.1.2.1.99.1.1.1.3",
			"1.3.6.1.2.1.99.1.1.1.4",
			"1.3.6.1.2.1.99.1.1.1.5",
			"1.3.6.1.2.1.99.1.1.1.6",
		},
	},
	"poe": {
		Tables: []string{
			"1.3.6.1.2.1.105.1.1.1.3",
			"1.3.6.1.2.1.105.1.1.1.4",
			"1.3.6.1.2.1.105.1.1.1.6",
			"1.3.6.1.2.1.105.1.1.1.7",
			"1.3.6.1.2.1.105.1.1.1.9",
			"1.3.6.1.2.1.105.1.1.1.10",
		},
	},
	"cpu_memory": {
		Scalars: []string{
			"1.3.6.1.2.1.25.2.2.0",
			"1.3.6.1.4.1.2021.4.5.0",
			"1.3.6.1.4.1.2021.4.6.0",
			"1.3.6.1.4.1.2021.11.9.0",
			"1.3.6.1.4.1.2021.11.10.0",
			"1.3.6.1.4.1.2021.11.11.0",
		},
		Tables: []string{
			"1.3.6.1.2.1.25.3.3.1.2",
			"1.3.6.1.2.1.25.2.3.1.3",
			"1.3.6.1.2.1.25.2.3.1.4",
			"1.3.6.1.2.1.25.2.3.1.5",
			"1.3.6.1.2.1.25.2.3.1.6",
		},
	},
}

var snmpSegmentDefaults = map[string]snmpSegmentDefault{
	snmpCollectionMetricsFast: {
		Groups:       []string{"system", "interfaces_status", "interfaces_counters", "ip_stats"},
		FetchMode:    snmpFetchAuto,
		MaxRows:      1000,
		TimeBudgetMs: 60000,
	},
	snmpCollectionSwitchportSlow: {
		Groups:       []string{"vlan"},
		FetchMode:    snmpFetchWalkOnly,
		MaxRows:      2048,
		TimeBudgetMs: 180000,
	},
	snmpCollectionDeepDiag: {
		Groups:       []string{"stp", "lldp_local", "lldp_remote", "poe", "entity", "sensors", "cpu_memory"},
		FetchMode:    snmpFetchAuto,
		MaxRows:      1000,
		TimeBudgetMs: 240000,
	},
}

var snmpProfileGroups = map[string][]string{
	"minimal": {
		"system",
		"interfaces_status",
		"cpu_memory",
	},
	"switch_noc": {
		"system",
		"interfaces_status",
		"interfaces_counters",
		"ip_stats",
		"bridge_fdb",
		"vlan",
		"stp",
		"lldp_local",
		"lldp_remote",
		"poe",
		"cpu_memory",
	},
	"full": {
		"system",
		"interfaces_status",
		"interfaces_counters",
		"ip_stats",
		"bridge_fdb",
		"vlan",
		"stp",
		"lldp_local",
		"lldp_remote",
		"entity",
		"sensors",
		"poe",
		"cpu_memory",
	},
}

func buildSNMPPlan(job entities.AgentlessJob, host string) snmpPlan {
	cfg := map[string]any(job.Config)
	kind := snmpCollectionKind(job)
	profile := normalizeSNMPProfile(getString(cfg, "collection_profile", defaultSNMPCollectionProfile))
	segmentDefaults := snmpSegmentDefaults[kind]
	defaultFetchMode := snmpFetchAuto
	defaultMaxRows := defaultSNMPMaxRows
	defaultTimeBudget := maxInt(job.TimeoutMs*3, defaultSNMPTimeBudgetMs)
	if segmentDefaults.TimeBudgetMs > 0 {
		defaultTimeBudget = segmentDefaults.TimeBudgetMs
	}
	if segmentDefaults.MaxRows > 0 {
		defaultMaxRows = segmentDefaults.MaxRows
	}
	if segmentDefaults.FetchMode != "" {
		defaultFetchMode = segmentDefaults.FetchMode
	}
	if job.SNMP != nil && job.SNMP.TimeBudgetMs > 0 {
		defaultTimeBudget = job.SNMP.TimeBudgetMs
	}

	fetchMode := parseSNMPFetchMode(getString(cfg, "fetch_mode", getString(cfg, "snmp_fetch_mode", string(defaultFetchMode))))
	maxRows := positiveIntOr(cfg, "snmp_max_rows", positiveIntOr(cfg, "max_rows_per_table", defaultMaxRows))
	timeBudget := positiveIntOr(cfg, "time_budget_ms", positiveIntOr(cfg, "snmp_time_budget_ms", defaultTimeBudget))
	groups := cloneStringSlice(snmpProfileGroups[profile])
	if len(segmentDefaults.Groups) > 0 {
		groups = cloneStringSlice(segmentDefaults.Groups)
	}
	if customGroups := filterKnownSNGroups(readStringSlice(configValue(cfg, "groups"))); len(customGroups) > 0 {
		groups = customGroups
	} else if includeGroups := filterKnownSNGroups(readStringSlice(configValue(cfg, "include_groups"))); len(includeGroups) > 0 {
		groups = mergeSNMPGroups(groups, includeGroups)
	}

	return snmpPlan{
		Host:              host,
		ProfileID:         job.SNMP.ProfileID,
		CollectionProfile: profile,
		CollectionKind:    kind,
		FetchMode:         fetchMode,
		MaxRows:           maxRows,
		TimeBudgetMs:      timeBudget,
		Groups:            groups,
		CustomGet:         normalizeOIDList(readCustomOIDList(cfg, "get")),
		CustomWalk:        normalizeOIDList(readCustomOIDList(cfg, "walk")),
	}
}

func normalizeSNMPProfile(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if _, ok := snmpProfileGroups[v]; ok {
		return v
	}
	return defaultSNMPCollectionProfile
}

func normalizeSNMPCollectionKind(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "-", "_")
	switch v {
	case snmpCollectionMetricsFast, snmpCollectionSwitchportSlow, snmpCollectionDeepDiag:
		return v
	default:
		return snmpCollectionLegacy
	}
}

func snmpCollectionKind(job entities.AgentlessJob) string {
	cfg := map[string]any(job.Config)
	return normalizeSNMPCollectionKind(firstNonEmptyString(
		job.CollectionKind,
		getString(cfg, "snmp_collection_kind", ""),
		getString(cfg, "collection_kind", ""),
		getString(cfg, "segment_id", ""),
	))
}

func parseSNMPFetchMode(v string) snmpFetchMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(snmpFetchGetOnly), "get":
		return snmpFetchGetOnly
	case string(snmpFetchWalkOnly), "walk":
		return snmpFetchWalkOnly
	case string(snmpFetchAuto), "mixed", "":
		return snmpFetchAuto
	default:
		return snmpFetchAuto
	}
}

func positiveIntOr(cfg map[string]any, key string, def int) int {
	v, ok := toInt(configValue(cfg, key))
	if !ok || v <= 0 {
		return def
	}
	return v
}

func configValue(cfg map[string]any, key string) any {
	if cfg == nil {
		return nil
	}
	if v, ok := cfg[key]; ok {
		return v
	}
	return nil
}

func readCustomOIDList(cfg map[string]any, key string) []string {
	var out []string
	if cfg == nil {
		return out
	}
	if raw, ok := cfg["custom"]; ok {
		if m, ok := raw.(map[string]any); ok {
			out = append(out, readStringSlice(m[key])...)
		}
	}
	out = append(out, readStringSlice(cfg["custom."+key])...)
	out = append(out, readStringSlice(cfg["custom_"+key])...)
	if key == "walk" {
		out = append(out, readNamedOIDList(cfg["custom_walk_oids"])...)
	}
	if key == "get" {
		out = append(out, readNamedOIDList(cfg["custom_get_oids"])...)
	}
	return out
}

func readNamedOIDList(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			switch x := item.(type) {
			case string:
				out = append(out, x)
			case map[string]any:
				if oid, ok := x["oid"].(string); ok {
					out = append(out, oid)
				}
			}
		}
		return out
	case []map[string]any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if oid, ok := item["oid"].(string); ok {
				out = append(out, oid)
			}
		}
		return out
	default:
		return readStringSlice(v)
	}
}

func readStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case []string:
		cp := make([]string, 0, len(t))
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				cp = append(cp, s)
			}
		}
		return cp
	case []any:
		cp := make([]string, 0, len(t))
		for _, item := range t {
			switch x := item.(type) {
			case string:
				s := strings.TrimSpace(x)
				if s != "" {
					cp = append(cp, s)
				}
			}
		}
		return cp
	case string:
		parts := strings.Split(t, ",")
		cp := make([]string, 0, len(parts))
		for _, p := range parts {
			s := strings.TrimSpace(p)
			if s != "" {
				cp = append(cp, s)
			}
		}
		return cp
	default:
		return nil
	}
}

func filterKnownSNGroups(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, g := range in {
		g = normalizeSNMPGroupName(g)
		if g == "" {
			continue
		}
		if _, ok := snmpGroupDefs[g]; !ok {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return out
}

func normalizeSNMPGroupName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "host_resources", "ucd_system":
		return "cpu_memory"
	default:
		return v
	}
}

func mergeSNMPGroups(base, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, group := range append(cloneStringSlice(base), extra...) {
		group = normalizeSNMPGroupName(group)
		if group == "" {
			continue
		}
		if _, ok := snmpGroupDefs[group]; !ok {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		out = append(out, group)
	}
	return out
}

func normalizeOIDList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, oid := range in {
		oid = strings.TrimSpace(oid)
		oid = strings.TrimPrefix(oid, ".")
		if oid == "" {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = struct{}{}
		out = append(out, oid)
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	cp := make([]string, len(in))
	copy(cp, in)
	return cp
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
