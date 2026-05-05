package agentless

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestBuildSNMPPlanSwitchNocFromConfig(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs: 2000,
		SNMP: &entities.AgentlessSnmpProfile{
			ProfileID: 42,
		},
		Config: entities.AgentlessConfig{
			"collection_profile": "switch_noc",
			"fetch_mode":         "walk_only",
			"snmp_max_rows":      80,
			"time_budget_ms":     9000,
			"custom": map[string]any{
				"get":  []any{"1.3.6.1.2.1.1.3.0"},
				"walk": []any{"1.3.6.1.2.1.2.2.1.2"},
			},
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")
	if plan.Host != "10.10.10.10" {
		t.Fatalf("host inesperado: %q", plan.Host)
	}
	if plan.ProfileID != 42 {
		t.Fatalf("profile_id inesperado: %d", plan.ProfileID)
	}
	if plan.CollectionProfile != "switch_noc" {
		t.Fatalf("perfil inesperado: %q", plan.CollectionProfile)
	}
	if plan.FetchMode != snmpFetchWalkOnly {
		t.Fatalf("fetch_mode inesperado: %q", plan.FetchMode)
	}
	if plan.MaxRows != 80 {
		t.Fatalf("snmp_max_rows inesperado: %d", plan.MaxRows)
	}
	if plan.TimeBudgetMs != 9000 {
		t.Fatalf("time_budget_ms inesperado: %d", plan.TimeBudgetMs)
	}
	if len(plan.Groups) == 0 {
		t.Fatalf("groups vazio")
	}
	if got := snmpOIDStrings(plan.CustomGet); !slices.Equal(got, []string{"1.3.6.1.2.1.1.3.0"}) {
		t.Fatalf("custom.get inesperado: %#v", plan.CustomGet)
	}
	if got := snmpOIDStrings(plan.CustomWalk); !slices.Equal(got, []string{"1.3.6.1.2.1.2.2.1.2"}) {
		t.Fatalf("custom.walk inesperado: %#v", plan.CustomWalk)
	}
}

func TestBuildSNMPPlanDefaults(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs: 1000,
		SNMP:      &entities.AgentlessSnmpProfile{},
		Config:    entities.AgentlessConfig{},
	}

	plan := buildSNMPPlan(job, "host.local")
	if plan.CollectionProfile != "switch_noc" {
		t.Fatalf("esperado switch_noc, obtido %q", plan.CollectionProfile)
	}
	if plan.FetchMode != snmpFetchAuto {
		t.Fatalf("esperado auto, obtido %q", plan.FetchMode)
	}
	if plan.MaxRows != defaultSNMPMaxRows {
		t.Fatalf("esperado max_rows=%d, obtido %d", defaultSNMPMaxRows, plan.MaxRows)
	}
	if plan.TimeBudgetMs <= 0 {
		t.Fatalf("time_budget_ms invalido: %d", plan.TimeBudgetMs)
	}
	if len(plan.Groups) == 0 {
		t.Fatalf("esperava grupos para perfil switch_noc")
	}
}

func TestBuildSNMPPlanMetricsFastDefaultUsaSystemCustom(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs:      2000,
		CollectionKind: "metrics_fast",
		SNMP:           &entities.AgentlessSnmpProfile{ProfileID: 42},
		Config: entities.AgentlessConfig{
			"collection_kind":      "metrics_fast",
			"snmp_collection_kind": "metrics_fast",
			"segment_id":           "metrics_fast",
			"collection_profile":   "minimal",
			"fetch_mode":           "auto",
			"time_budget_ms":       60000,
			"snmp_max_rows":        1000,
			"include_groups":       []any{"system", "custom"},
			"exclude_groups":       []any{"interfaces_status", "interfaces_counters", "ip_stats", "vlan", "bridge_fdb"},
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")
	if plan.CollectionKind != "metrics_fast" {
		t.Fatalf("collection_kind inesperado: %q", plan.CollectionKind)
	}
	if plan.CollectionProfile != "minimal" {
		t.Fatalf("collection_profile inesperado: %q", plan.CollectionProfile)
	}
	if plan.FetchMode != snmpFetchAuto {
		t.Fatalf("fetch_mode inesperado: %q", plan.FetchMode)
	}
	if plan.TimeBudgetMs != 60000 {
		t.Fatalf("time_budget_ms inesperado: %d", plan.TimeBudgetMs)
	}
	if plan.MaxRows != 1000 {
		t.Fatalf("snmp_max_rows inesperado: %d", plan.MaxRows)
	}
	if !slices.Equal(plan.Groups, []string{"system", "custom"}) {
		t.Fatalf("groups deveriam respeitar system/custom, obtido %#v", plan.Groups)
	}
}

func TestBuildSNMPPlanRuntimeTopLevelTemPrecedenciaSobreConfig(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs:           2000,
		CollectionKind:      "metrics_fast",
		CollectionKindAlias: "metrics_fast",
		SegmentID:           "metrics_fast",
		CollectionProfile:   "minimal",
		FetchMode:           "get",
		TimeBudgetMs:        45000,
		SNMPMaxRows:         150,
		IncludeGroups:       []string{"system", "custom"},
		ExcludeGroups:       []string{"interfaces_status", "interfaces_counters"},
		SNMP:                &entities.AgentlessSnmpProfile{ProfileID: 42, TimeBudgetMs: 120000},
		Config: entities.AgentlessConfig{
			"collection_kind":    "deep_diag",
			"collection_profile": "full",
			"fetch_mode":         "walk",
			"time_budget_ms":     240000,
			"snmp_max_rows":      5000,
			"include_groups":     []any{"stp", "lldp_remote", "entity"},
			"exclude_groups":     []any{"system"},
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")

	if plan.CollectionKind != "metrics_fast" || plan.SegmentID != "metrics_fast" {
		t.Fatalf("runtime top-level nao prevaleceu: kind=%q segment=%q", plan.CollectionKind, plan.SegmentID)
	}
	if plan.CollectionProfile != "minimal" {
		t.Fatalf("collection_profile inesperado: %q", plan.CollectionProfile)
	}
	if plan.FetchMode != snmpFetchGetOnly {
		t.Fatalf("fetch_mode inesperado: %q", plan.FetchMode)
	}
	if plan.TimeBudgetMs != 45000 || plan.MaxRows != 150 {
		t.Fatalf("limites inesperados: budget=%d rows=%d", plan.TimeBudgetMs, plan.MaxRows)
	}
	if !slices.Equal(plan.IncludeGroups, []string{"system", "custom"}) {
		t.Fatalf("include_groups normalizados inesperados: %#v", plan.IncludeGroups)
	}
	if !slices.Equal(plan.Groups, []string{"system", "custom"}) {
		t.Fatalf("groups finais inesperados: %#v", plan.Groups)
	}
}

func TestBuildSNMPPlanFallbackGroupsNaoAmpliaIncludeGroupsExplicito(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs:      2000,
		CollectionKind: "metrics_fast",
		SNMP:           &entities.AgentlessSnmpProfile{ProfileID: 42},
		Config: entities.AgentlessConfig{
			"collection_profile": "minimal",
			"fetch_mode":         "auto",
			"include_groups":     []any{"system", "custom"},
			"fallback_groups":    []any{"interfaces_status", "interfaces_counters", "ip_stats", "cpu_memory"},
			"time_budget_ms":     60000,
			"snmp_max_rows":      1000,
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")

	if !slices.Equal(plan.Groups, []string{"system", "custom"}) {
		t.Fatalf("fallback_groups ampliou grupos explicitos: %#v", plan.Groups)
	}
	if plan.FallbackApplied {
		t.Fatalf("fallback_groups nao deveria ser aplicado com include_groups explicito")
	}
	if !slices.Equal(plan.FallbackGroups, []string{"interfaces_status", "interfaces_counters", "ip_stats", "cpu_memory"}) {
		t.Fatalf("fallback_groups deveria ficar apenas registrado: %#v", plan.FallbackGroups)
	}
}

func TestBuildSNMPPlanFixtureALEOmniSwitchMetricsFastRuntime(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "pkg38_ale_omniswitch_runtime_job.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var job entities.AgentlessJob
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	plan := buildSNMPPlan(job, "192.0.2.85")

	if plan.CollectionKind != snmpCollectionMetricsFast || plan.SegmentID != snmpCollectionMetricsFast {
		t.Fatalf("segmento inesperado: kind=%q segment=%q", plan.CollectionKind, plan.SegmentID)
	}
	if plan.CollectionProfile != "minimal" || plan.FetchMode != snmpFetchAuto {
		t.Fatalf("runtime inesperado: profile=%q mode=%q", plan.CollectionProfile, plan.FetchMode)
	}
	if plan.TimeBudgetMs != 60000 || plan.MaxRows != 1000 {
		t.Fatalf("limites inesperados: budget=%d rows=%d", plan.TimeBudgetMs, plan.MaxRows)
	}
	if !slices.Equal(plan.Groups, []string{"system", "custom"}) {
		t.Fatalf("fixture metrics_fast deve executar apenas system/custom, obtido %#v", plan.Groups)
	}
	if plan.FallbackApplied {
		t.Fatalf("fallback_groups nao pode ampliar fixture com include_groups explicito")
	}
	if !slices.Equal(plan.FallbackGroups, []string{"interfaces_status", "interfaces_counters", "ip_stats", "cpu_memory"}) {
		t.Fatalf("fallback_groups deve ficar apenas diagnosticado: %#v", plan.FallbackGroups)
	}
	if plan.VendorProfile == nil || plan.VendorProfile.ProfileKey != "ale_omniswitch_aos_health_v1" {
		t.Fatalf("vendor profile ALE/OmniSwitch ausente: %#v", plan.VendorProfile)
	}
	if got := snmpOIDStrings(plan.CustomWalk); !slices.Equal(got, []string{
		"1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15",
		"1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.16",
	}) {
		t.Fatalf("fixture ALE/OmniSwitch perdeu OIDs proprietarios: %#v", got)
	}
}

func TestBuildSNMPPlanSwitchportSegmentFromBackendConfig(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs:      2000,
		CollectionKind: "switchport_slow",
		SNMP: &entities.AgentlessSnmpProfile{
			ProfileID: 42,
		},
		Config: entities.AgentlessConfig{
			"collection_profile": "minimal",
			"fetch_mode":         "walk",
			"include_groups":     []any{"vlan", "bridge_fdb"},
			"time_budget_ms":     180000,
			"snmp_max_rows":      2048,
			"custom_walk_oids": []any{
				map[string]any{"name": "dot1d_base_port_if_index", "oid": "1.3.6.1.2.1.17.1.4.1.2"},
				map[string]any{"name": "dot1q_pvid", "oid": "1.3.6.1.2.1.17.7.1.4.5.1.1"},
			},
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")
	if plan.CollectionKind != "switchport_slow" {
		t.Fatalf("collection_kind inesperado: %q", plan.CollectionKind)
	}
	if plan.FetchMode != snmpFetchWalkOnly {
		t.Fatalf("fetch_mode inesperado: %q", plan.FetchMode)
	}
	if plan.MaxRows != 2048 {
		t.Fatalf("snmp_max_rows inesperado: %d", plan.MaxRows)
	}
	if plan.TimeBudgetMs != 180000 {
		t.Fatalf("time_budget_ms inesperado: %d", plan.TimeBudgetMs)
	}
	if !slices.Equal(plan.Groups, []string{"vlan", "bridge_fdb"}) {
		t.Fatalf("groups inesperado: %#v", plan.Groups)
	}
	if got := snmpOIDStrings(plan.CustomWalk); !slices.Equal(got, []string{"1.3.6.1.2.1.17.1.4.1.2", "1.3.6.1.2.1.17.7.1.4.5.1.1"}) {
		t.Fatalf("custom_walk inesperado: %#v", plan.CustomWalk)
	}
}

func TestBuildSNMPPlanIncludeGroupsSubstituiPerfilEExcludeRemoveGrupos(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs:      2000,
		CollectionKind: "metrics_fast",
		SNMP: &entities.AgentlessSnmpProfile{
			ProfileID:    19,
			TimeBudgetMs: 15000,
		},
		Config: entities.AgentlessConfig{
			"collection_profile": "switch_noc",
			"fetch_mode":         "auto",
			"include_groups": []any{
				"system",
				"interfaces_status",
				"cpu_memory",
				"ip_stats",
				"custom",
			},
			"exclude_groups": []any{
				"interfaces_status",
				"interfaces_counters",
				"vlan",
				"bridge_fdb",
				"stp",
				"lldp_local",
				"lldp_remote",
				"poe",
				"entity",
				"sensors",
				"host_resources",
				"ucd_system",
			},
			"snmp_max_rows":  1000,
			"time_budget_ms": 15000,
			"custom_get_oids": []any{
				map[string]any{"name": "health_module_cpu_latest_slot_1", "oid": "1.3.6.1.4.1.6486.800.1.2.1.16.1.1.2.1.1.14.1"},
				map[string]any{"name": "health_module_memory_latest_slot_1", "oid": "1.3.6.1.4.1.6486.800.1.2.1.16.1.1.2.1.1.10.1"},
			},
		},
	}

	plan := buildSNMPPlan(job, "10.209.7.20")
	if plan.CollectionProfile != "switch_noc" {
		t.Fatalf("collection_profile inesperado: %q", plan.CollectionProfile)
	}
	if !slices.Equal(plan.Groups, []string{"system", "ip_stats", "custom"}) {
		t.Fatalf("groups deveriam respeitar include/exclude, obtido %#v", plan.Groups)
	}
	if plan.TimeBudgetMs != 15000 {
		t.Fatalf("time_budget_ms inesperado: %d", plan.TimeBudgetMs)
	}
	if got := snmpOIDStrings(plan.CustomGet); !slices.Equal(got, []string{
		"1.3.6.1.4.1.6486.800.1.2.1.16.1.1.2.1.1.14.1",
		"1.3.6.1.4.1.6486.800.1.2.1.16.1.1.2.1.1.10.1",
	}) {
		t.Fatalf("custom_get_oids inesperado: %#v", got)
	}
}

func TestBuildSNMPPlanVendorOIDProfilePreservaMetadadosEOrdem(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs: 2000,
		SNMP:      &entities.AgentlessSnmpProfile{ProfileID: 42},
		Config: entities.AgentlessConfig{
			"vendor_oid_profile": map[string]any{
				"profile_key":       "ale_oaw_ap_base_v1",
				"profile_version":   "1.0.0",
				"vendor":            "Alcatel-Lucent Enterprise",
				"family":            "OmniAccess OAW AP",
				"model":             "OAW-AP1232",
				"source_mib":        "OAW-APxxxx + ALCATEL-NGOAW-DEVICES-MIB",
				"matched_by":        "sysObjectID",
				"match_source":      "config_json.applied",
				"last_validated_at": "2026-05-04 12:00:00",
				"applied":           true,
			},
			"custom_get_oids": []any{
				map[string]any{
					"name":          "ap_cpu_utilization",
					"oid":           ".1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0",
					"label":         "CPU do AP",
					"metric":        "apCpuUtilization",
					"source_mib":    "OAW-APxxxx",
					"unit":          "percent",
					"canonical_key": "cpu_percent",
				},
				map[string]any{
					"name":          "ap_memory_utilization",
					"oid":           "1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0",
					"metric":        "apMemoryUtilization",
					"source_mib":    "OAW-APxxxx",
					"unit":          "percent",
					"canonical_key": "memory_percent",
				},
			},
			"custom_walk_oids": []any{
				map[string]any{
					"name":          "health_module_cpu_latest",
					"oid":           "1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15",
					"metric":        "healthModuleCpuLatest",
					"source_mib":    "ALCATEL-IND1-HEALTH-MIB",
					"unit":          "percent",
					"canonical_key": "cpu_percent",
				},
			},
			"fallback_groups": []any{"system", "interfaces_status", "cpu_memory"},
		},
	}

	plan := buildSNMPPlan(job, "10.10.10.10")
	if plan.VendorProfile == nil {
		t.Fatalf("vendor profile ausente")
	}
	if plan.VendorProfile.ProfileKey != "ale_oaw_ap_base_v1" || !plan.VendorProfile.Applied {
		t.Fatalf("vendor profile inesperado: %#v", plan.VendorProfile)
	}
	if got := snmpOIDStrings(plan.CustomGet); !slices.Equal(got, []string{
		"1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.8.0",
		"1.3.6.1.4.1.6486.802.1.1.2.1.2.5.1.1.7.0",
	}) {
		t.Fatalf("custom_get_oids inesperado: %#v", got)
	}
	cpu := plan.CustomGet[0]
	if cpu.Name != "ap_cpu_utilization" || cpu.Label != "CPU do AP" || cpu.Metric != "apCpuUtilization" || cpu.SourceMIB != "OAW-APxxxx" || cpu.Unit != "percent" || cpu.CanonicalKey != "cpu_percent" {
		t.Fatalf("metadados do OID custom_get perdidos: %#v", cpu)
	}
	if got := snmpOIDStrings(plan.CustomWalk); !slices.Equal(got, []string{"1.3.6.1.4.1.6486.801.1.2.1.16.1.1.1.1.1.15"}) {
		t.Fatalf("custom_walk_oids inesperado: %#v", got)
	}
	walk := plan.CustomWalk[0]
	if walk.Name != "health_module_cpu_latest" || walk.Metric != "healthModuleCpuLatest" || walk.SourceMIB != "ALCATEL-IND1-HEALTH-MIB" || walk.CanonicalKey != "cpu_percent" {
		t.Fatalf("metadados do OID custom_walk perdidos: %#v", walk)
	}
}

func TestBuildSNMPPlanSemPerfilMantemFallbackGenerico(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs: 1000,
		SNMP:      &entities.AgentlessSnmpProfile{},
		Config:    entities.AgentlessConfig{},
	}

	plan := buildSNMPPlan(job, "host.local")
	if plan.VendorProfile != nil {
		t.Fatalf("vendor profile nao deveria existir: %#v", plan.VendorProfile)
	}
	if len(plan.CustomGet) != 0 || len(plan.CustomWalk) != 0 {
		t.Fatalf("custom OIDs nao deveriam existir sem perfil: get=%#v walk=%#v", plan.CustomGet, plan.CustomWalk)
	}
	if len(plan.Groups) == 0 {
		t.Fatalf("fallback generico sem grupos")
	}
}

func TestBuildSNMPPlanCustomOIDsLegadosContinuamCompatíveis(t *testing.T) {
	job := entities.AgentlessJob{
		TimeoutMs: 1000,
		SNMP:      &entities.AgentlessSnmpProfile{},
		Config: entities.AgentlessConfig{
			"custom_get_oids":  []any{"1.3.6.1.2.1.1.1.0"},
			"custom_walk_oids": []any{"1.3.6.1.2.1.2.2.1.2"},
		},
	}

	plan := buildSNMPPlan(job, "host.local")
	if plan.VendorProfile != nil {
		t.Fatalf("vendor profile nao deveria existir no formato legado: %#v", plan.VendorProfile)
	}
	if got := snmpOIDStrings(plan.CustomGet); !slices.Equal(got, []string{"1.3.6.1.2.1.1.1.0"}) {
		t.Fatalf("custom_get legado inesperado: %#v", got)
	}
	if got := snmpOIDStrings(plan.CustomWalk); !slices.Equal(got, []string{"1.3.6.1.2.1.2.2.1.2"}) {
		t.Fatalf("custom_walk legado inesperado: %#v", got)
	}
	if plan.CustomGet[0].Name != "" || plan.CustomGet[0].CanonicalKey != "" || plan.CustomWalk[0].SourceMIB != "" {
		t.Fatalf("metadados nao deveriam ser inventados no formato legado: get=%#v walk=%#v", plan.CustomGet[0], plan.CustomWalk[0])
	}
}

func TestSNMPVlanGroupIncludesBridgePortIfIndex(t *testing.T) {
	group, ok := snmpGroupDefs["vlan"]
	if !ok {
		t.Fatalf("grupo vlan ausente")
	}
	const wantOID = "1.3.6.1.2.1.17.1.4.1.2"
	if !slices.Contains(group.Tables, wantOID) {
		t.Fatalf("OID %s ausente no grupo vlan: %#v", wantOID, group.Tables)
	}
}

func TestSNMPVlanGroupPrioritizesSwitchportModeOIDs(t *testing.T) {
	group, ok := snmpGroupDefs["vlan"]
	if !ok {
		t.Fatalf("grupo vlan ausente")
	}
	want := []string{
		"1.3.6.1.2.1.17.1.4.1.2",
		"1.3.6.1.2.1.17.7.1.4.2.1.4",
		"1.3.6.1.2.1.17.7.1.4.2.1.5",
		"1.3.6.1.2.1.17.7.1.4.5.1.1",
	}
	if len(group.Tables) < len(want) || !slices.Equal(group.Tables[:len(want)], want) {
		t.Fatalf("ordem obrigatoria inesperada: %#v", group.Tables)
	}
}
