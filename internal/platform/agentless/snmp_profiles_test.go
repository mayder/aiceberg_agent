package agentless

import (
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
	if len(plan.CustomGet) != 1 || plan.CustomGet[0] != "1.3.6.1.2.1.1.3.0" {
		t.Fatalf("custom.get inesperado: %#v", plan.CustomGet)
	}
	if len(plan.CustomWalk) != 1 || plan.CustomWalk[0] != "1.3.6.1.2.1.2.2.1.2" {
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
	if !slices.Equal(plan.CustomWalk, []string{"1.3.6.1.2.1.17.1.4.1.2", "1.3.6.1.2.1.17.7.1.4.5.1.1"}) {
		t.Fatalf("custom_walk inesperado: %#v", plan.CustomWalk)
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
