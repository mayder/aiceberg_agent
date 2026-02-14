package agentless

import (
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
	if plan.CollectionProfile != "minimal" {
		t.Fatalf("esperado minimal, obtido %q", plan.CollectionProfile)
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
		t.Fatalf("esperava grupos para perfil minimal")
	}
}
