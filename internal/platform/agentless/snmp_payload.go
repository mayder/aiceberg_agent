package agentless

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

type snmpStats struct {
	GetAttempts  int `json:"get_attempts"`
	GetSuccess   int `json:"get_success"`
	WalkAttempts int `json:"walk_attempts"`
	WalkRows     int `json:"walk_rows"`
}

type snmpGroupStats struct {
	GetAttempts  int      `json:"get_attempts"`
	GetSuccess   int      `json:"get_success"`
	WalkAttempts int      `json:"walk_attempts"`
	WalkRows     int      `json:"walk_rows"`
	LatencyMs    int      `json:"latency_ms,omitempty"`
	Errors       []string `json:"errors,omitempty"`
}

type snmpRuntimeApplied struct {
	SNMPCollectionKind string   `json:"snmp_collection_kind"`
	CollectionKind     string   `json:"collection_kind"`
	SegmentID          string   `json:"segment_id"`
	CollectionProfile  string   `json:"collection_profile"`
	FetchMode          string   `json:"fetch_mode"`
	TimeBudgetMs       int      `json:"time_budget_ms"`
	SNMPMaxRows        int      `json:"snmp_max_rows"`
	IncludeGroups      []string `json:"include_groups"`
	ExcludeGroups      []string `json:"exclude_groups"`
	GroupsRequested    []string `json:"groups_requested"`
	FallbackGroups     []string `json:"fallback_groups,omitempty"`
	FallbackApplied    bool     `json:"fallback_applied"`
}

type snmpPayload struct {
	Host                 string                                  `json:"host"`
	ProfileID            int                                     `json:"profile_id"`
	SNMPCollectionKind   string                                  `json:"snmp_collection_kind"`
	CollectionProfile    string                                  `json:"collection_profile"`
	CollectionKind       string                                  `json:"collection_kind"`
	SegmentID            string                                  `json:"segment_id"`
	FetchMode            string                                  `json:"fetch_mode"`
	TimeBudgetMs         int                                     `json:"time_budget_ms"`
	SNMPMaxRows          int                                     `json:"snmp_max_rows"`
	TimeBudgetExceeded   bool                                    `json:"time_budget_exceeded"`
	RuntimeApplied       snmpRuntimeApplied                      `json:"runtime_applied"`
	VendorOIDProfile     *entities.AgentlessSnmpVendorOIDProfile `json:"vendor_oid_profile,omitempty"`
	VendorProfileApplied bool                                    `json:"vendor_profile_applied"`
	ProfileKey           string                                  `json:"profile_key,omitempty"`
	ProfileVersion       string                                  `json:"profile_version,omitempty"`
	CPUPercent           *float64                                `json:"cpu_percent,omitempty"`
	MemoryPercent        *float64                                `json:"memory_percent,omitempty"`
	CPUMemory            map[string]any                          `json:"cpu_memory,omitempty"`
	OIDsSuccess          []map[string]any                        `json:"oids_success"`
	OIDsFailed           []map[string]any                        `json:"oids_failed"`
	FallbackUsed         bool                                    `json:"fallback_used"`
	GroupsRequested      []string                                `json:"groups_requested"`
	Stats                snmpStats                               `json:"stats"`
	GroupStats           map[string]snmpGroupStats               `json:"group_stats"`
	Errors               []string                                `json:"errors"`
	Sys                  map[string]any                          `json:"sys"`
	Ifaces               []map[string]any                        `json:"ifaces"`
	Scalars              map[string]any                          `json:"scalars"`
	Tables               map[string][]map[string]any             `json:"tables"`
	Custom               map[string][]map[string]any             `json:"custom"`
	OIDs                 map[string]any                          `json:"oids,omitempty"`
}

func newSNMPPayload(plan snmpPlan) *snmpPayload {
	runtime := snmpRuntimeApplied{
		SNMPCollectionKind: plan.CollectionKind,
		CollectionKind:     plan.CollectionKind,
		SegmentID:          plan.SegmentID,
		CollectionProfile:  plan.CollectionProfile,
		FetchMode:          string(plan.FetchMode),
		TimeBudgetMs:       plan.TimeBudgetMs,
		SNMPMaxRows:        plan.MaxRows,
		IncludeGroups:      cloneStringSlice(plan.IncludeGroups),
		ExcludeGroups:      cloneStringSlice(plan.ExcludeGroups),
		GroupsRequested:    cloneStringSlice(plan.Groups),
		FallbackGroups:     cloneStringSlice(plan.FallbackGroups),
		FallbackApplied:    plan.FallbackApplied,
	}

	return &snmpPayload{
		Host:               plan.Host,
		ProfileID:          plan.ProfileID,
		SNMPCollectionKind: plan.CollectionKind,
		CollectionProfile:  plan.CollectionProfile,
		CollectionKind:     plan.CollectionKind,
		SegmentID:          plan.SegmentID,
		FetchMode:          string(plan.FetchMode),
		TimeBudgetMs:       plan.TimeBudgetMs,
		SNMPMaxRows:        plan.MaxRows,
		RuntimeApplied:     runtime,
		VendorOIDProfile:   plan.VendorProfile,
		ProfileKey:         snmpVendorProfileKey(plan),
		ProfileVersion:     snmpVendorProfileVersion(plan),
		OIDsSuccess:        []map[string]any{},
		OIDsFailed:         []map[string]any{},
		FallbackUsed:       plan.VendorProfile != nil && len(plan.Groups) > 0,
		GroupsRequested:    cloneStringSlice(plan.Groups),
		GroupStats:         make(map[string]snmpGroupStats),
		Errors:             []string{},
		Sys:                map[string]any{},
		Ifaces:             []map[string]any{},
		Scalars:            map[string]any{},
		Tables:             map[string][]map[string]any{},
		Custom: map[string][]map[string]any{
			"get":  {},
			"walk": {},
		},
		OIDs: map[string]any{},
	}
}

func snmpVendorProfileKey(plan snmpPlan) string {
	if plan.VendorProfile == nil {
		return ""
	}
	return plan.VendorProfile.ProfileKey
}

func snmpVendorProfileVersion(plan snmpPlan) string {
	if plan.VendorProfile == nil {
		return ""
	}
	return plan.VendorProfile.ProfileVersion
}

func (s *snmpStats) AddGroup(g snmpGroupStats) {
	s.GetAttempts += g.GetAttempts
	s.GetSuccess += g.GetSuccess
	s.WalkAttempts += g.WalkAttempts
	s.WalkRows += g.WalkRows
}

func (p *snmpPayload) addGroupStats(group string, g snmpGroupStats) {
	p.GroupStats[group] = g
	p.Stats.AddGroup(g)
}

func (p *snmpPayload) addError(err string) {
	err = strings.TrimSpace(err)
	if err == "" {
		return
	}
	p.Errors = append(p.Errors, err)
}

func (p *snmpPayload) addOIDSuccess(spec entities.AgentlessSnmpOIDSpec, fields map[string]any) {
	item := snmpOIDResultItem(spec)
	for key, value := range fields {
		item[key] = value
	}
	p.OIDsSuccess = append(p.OIDsSuccess, item)
	if p.VendorOIDProfile != nil {
		p.VendorProfileApplied = true
	}
	p.applyCanonicalOIDValue(spec, item)
}

func (p *snmpPayload) addOIDFailed(spec entities.AgentlessSnmpOIDSpec, err string) {
	item := snmpOIDResultItem(spec)
	item["error"] = strings.TrimSpace(err)
	p.OIDsFailed = append(p.OIDsFailed, item)
}

func snmpOIDResultItem(spec entities.AgentlessSnmpOIDSpec) map[string]any {
	item := map[string]any{"oid": spec.OID}
	if spec.Name != "" {
		item["name"] = spec.Name
	}
	if spec.Label != "" {
		item["label"] = spec.Label
	}
	if spec.Metric != "" {
		item["metric"] = spec.Metric
	}
	if spec.SourceMIB != "" {
		item["source_mib"] = spec.SourceMIB
	}
	if spec.Unit != "" {
		item["unit"] = spec.Unit
	}
	if spec.CanonicalKey != "" {
		item["canonical_key"] = spec.CanonicalKey
	}
	return item
}

func (p *snmpPayload) applyCanonicalOIDValue(spec entities.AgentlessSnmpOIDSpec, item map[string]any) {
	key := strings.TrimSpace(spec.CanonicalKey)
	if key == "" {
		return
	}

	value, ok := snmpCanonicalNumericValue(item)
	if !ok {
		return
	}

	switch key {
	case "cpu_percent":
		percent := snmpClampPercent(value)
		p.CPUPercent = &percent
		p.ensureCPUMemory()["cpu_percent"] = percent
		p.addCPUMemoryVendorSample(spec, percent)
	case "memory_percent":
		percent := snmpClampPercent(value)
		p.MemoryPercent = &percent
		p.ensureCPUMemory()["memory_percent"] = percent
		p.addCPUMemoryVendorSample(spec, percent)
	default:
		if strings.HasPrefix(key, "cpu_memory.vendor_samples.") {
			p.addCPUMemoryVendorSample(spec, value)
		}
	}
}

func (p *snmpPayload) ensureCPUMemory() map[string]any {
	if p.CPUMemory == nil {
		p.CPUMemory = map[string]any{}
	}
	if _, ok := p.CPUMemory["vendor_samples"]; !ok {
		p.CPUMemory["vendor_samples"] = []map[string]any{}
	}
	if _, ok := p.CPUMemory["source_profile"]; !ok && p.ProfileKey != "" {
		p.CPUMemory["source_profile"] = p.ProfileKey
	}
	return p.CPUMemory
}

func (p *snmpPayload) addCPUMemoryVendorSample(spec entities.AgentlessSnmpOIDSpec, value float64) {
	cpuMemory := p.ensureCPUMemory()
	sample := snmpOIDResultItem(spec)
	sample["value"] = value
	if p.ProfileKey != "" {
		sample["profile_key"] = p.ProfileKey
	}
	samples, _ := cpuMemory["vendor_samples"].([]map[string]any)
	samples = append(samples, sample)
	cpuMemory["vendor_samples"] = samples
}

func snmpCanonicalNumericValue(item map[string]any) (float64, bool) {
	if v, ok := snmpNumericValue(item["value"]); ok {
		return v, true
	}
	if rows, ok := item["data"]; ok {
		if v, ok := snmpFirstRowNumericValue(rows); ok {
			return v, true
		}
	}
	return snmpFirstRowNumericValue(item["rows"])
}

func snmpFirstRowNumericValue(rows any) (float64, bool) {
	switch typed := rows.(type) {
	case []map[string]any:
		for _, row := range typed {
			if v, ok := snmpNumericValue(row["value"]); ok {
				return v, true
			}
		}
	case []any:
		for _, raw := range typed {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := snmpNumericValue(row["value"]); ok {
				return v, true
			}
		}
	}
	return 0, false
}

func snmpNumericValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		parsed, err := v.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func snmpClampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Max(0, math.Min(100, value))
}

func (p *snmpPayload) toMap() map[string]any {
	raw, _ := json.Marshal(p)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func evaluateSNMPStatus(p *snmpPayload) (status, code, message string) {
	if p.Stats.GetSuccess > 0 || p.Stats.WalkRows > 0 {
		return "ok", "", "SNMP OK"
	}
	if p.TimeBudgetExceeded {
		return "fail", "snmp_time_budget_exceeded", "Coleta SNMP interrompida: budget excedido"
	}
	for _, msg := range p.Errors {
		l := strings.ToLower(msg)
		switch {
		case strings.Contains(l, "timeout"), strings.Contains(l, "no response"), strings.Contains(l, "i/o timeout"):
			return "fail", "snmp_timeout", fmt.Sprintf("SNMP sem resposta (timeout). Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "authentication"), strings.Contains(l, "authorization"), strings.Contains(l, "unknown username"), strings.Contains(l, "wrongdigest"), strings.Contains(l, "decryption"):
			return "fail", "snmp_auth", fmt.Sprintf("Falha de credencial/versao SNMP. Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "connection refused"):
			return "fail", "snmp_refused", fmt.Sprintf("SNMP desabilitado ou porta bloqueada. Detalhe: %s", trimString(msg, 180))
		case strings.Contains(l, "tabela sem retorno"):
			return "fail", "snmp_table_empty", fmt.Sprintf("Tabela SNMP sem retorno. Detalhe: %s", trimString(msg, 180))
		}
	}
	if len(p.Errors) > 0 {
		return "fail", "snmp_fail", trimString(p.Errors[0], 255)
	}
	return "fail", "snmp_no_data", "SNMP sem dados uteis"
}
