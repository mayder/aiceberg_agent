package agentless

import "testing"

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
