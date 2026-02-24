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

type snmpGroupDef struct {
	Scalars []string
	Tables  []string
}

type snmpPlan struct {
	Host              string
	ProfileID         int
	CollectionProfile string
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
			"1.3.6.1.2.1.17.7.1.4.3.1.1",
			"1.3.6.1.2.1.17.7.1.4.5.1.1",
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
	profile := normalizeSNMPProfile(getString(cfg, "collection_profile", defaultSNMPCollectionProfile))
	fetchMode := parseSNMPFetchMode(getString(cfg, "fetch_mode", string(snmpFetchAuto)))
	maxRows := positiveIntOr(cfg, "snmp_max_rows", defaultSNMPMaxRows)
	timeBudget := positiveIntOr(cfg, "time_budget_ms", maxInt(job.TimeoutMs*3, defaultSNMPTimeBudgetMs))
	groups := cloneStringSlice(snmpProfileGroups[profile])
	if customGroups := filterKnownSNGroups(readStringSlice(configValue(cfg, "groups"))); len(customGroups) > 0 {
		groups = customGroups
	}

	return snmpPlan{
		Host:              host,
		ProfileID:         job.SNMP.ProfileID,
		CollectionProfile: profile,
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

func parseSNMPFetchMode(v string) snmpFetchMode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(snmpFetchGetOnly):
		return snmpFetchGetOnly
	case string(snmpFetchWalkOnly):
		return snmpFetchWalkOnly
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
	return out
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
		g = strings.ToLower(strings.TrimSpace(g))
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
