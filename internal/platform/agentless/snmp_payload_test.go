package agentless

import (
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
)

func TestSNMPManualDedupeUsesCommandAndPreservesRoutineIdentity(t *testing.T) {
	at := time.Date(2026, 11, 23, 18, 39, 0, 0, time.UTC)
	obs := entities.AgentlessObservation{CheckID: 91, ObservedAt: at, CollectionKind: "legacy"}
	if key := observationDedupeKey(&obs, "", 1); key != "check:91|observed:20261123183900|segment:legacy|seq:1" {
		t.Fatalf("routine dedupe contract changed: %s", key)
	}
	obs.CommandID = "command-one"
	first := observationDedupeKey(&obs, "", 1)
	obs.ObservedAt = at.Add(time.Second)
	if retry := observationDedupeKey(&obs, "", 1); retry != first {
		t.Fatal("retry changed the command identity")
	}
	obs.ObservedAt, obs.CommandID = at, "command-two"
	if second := observationDedupeKey(&obs, "", 1); second == first {
		t.Fatal("different commands in the same second shared a dedupe key")
	}
}

func TestSNMPStatsAddGroup(t *testing.T) {
	var stats snmpStats
	stats.AddGroup(snmpGroupStats{GetAttempts: 2, GetSuccess: 1, WalkAttempts: 3, WalkRows: 10})
	stats.AddGroup(snmpGroupStats{GetAttempts: 1, GetSuccess: 1, WalkAttempts: 2, WalkRows: 5})

	if stats.GetAttempts != 3 {
		t.Fatalf("get_attempts inesperado: %d", stats.GetAttempts)
	}
	if stats.GetSuccess != 2 {
		t.Fatalf("get_success inesperado: %d", stats.GetSuccess)
	}
	if stats.WalkAttempts != 5 {
		t.Fatalf("walk_attempts inesperado: %d", stats.WalkAttempts)
	}
	if stats.WalkRows != 15 {
		t.Fatalf("walk_rows inesperado: %d", stats.WalkRows)
	}
}

func TestEvaluateSNMPStatus(t *testing.T) {
	okPayload := newSNMPPayload(snmpPlan{
		Host:              "10.0.0.1",
		CollectionProfile: "minimal",
		FetchMode:         snmpFetchAuto,
		TimeBudgetMs:      2000,
		Groups:            []string{"system"},
	})
	okPayload.Stats.GetSuccess = 1
	status, code, message := evaluateSNMPStatus(okPayload)
	if status != "ok" || code != "" || message != "SNMP OK" {
		t.Fatalf("status ok inesperado: status=%q code=%q message=%q", status, code, message)
	}

	failPayload := newSNMPPayload(snmpPlan{
		Host:              "10.0.0.2",
		CollectionProfile: "minimal",
		FetchMode:         snmpFetchAuto,
		TimeBudgetMs:      2000,
		Groups:            []string{"system"},
	})
	failPayload.addError("request timeout no response")
	status, code, message = evaluateSNMPStatus(failPayload)
	if status != "fail" {
		t.Fatalf("status fail esperado, obtido %q", status)
	}
	if code != "snmp_timeout" {
		t.Fatalf("code esperado snmp_timeout, obtido %q", code)
	}
	if message == "" {
		t.Fatalf("mensagem de erro vazia")
	}
}
