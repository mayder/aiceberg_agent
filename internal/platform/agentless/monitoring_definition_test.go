package agentless

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func TestSNMPDefinitionUsesResolvedAuthorityAndPreservesLegacyJob(t *testing.T) {
	job := newSNMPJob()
	job.CheckID = 72
	job.ClienteID = 3
	job.AtivoID = 52
	job.SNMP.Community = "fixture-definition-private"
	job.Endpoint = &entities.AgentlessEndpoint{Endereco: "192.0.2.20"}
	job.Config["freshness_sec"] = 1
	authority := monitoring.DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{72}}
	policy := SNMPDefinitionPolicy{Revision: 1, FreshnessSec: 60, HistoryDays: 30, TrendsDays: 365}
	before, _ := json.Marshal(job)
	d, err := DescribeSNMPItems(job, authority, policy)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(d)
	for _, private := range []string{"fixture-definition-private", "192.0.2.20", "community", "endpoint"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("definition copied a private job field")
		}
	}
	observation := entities.AgentlessObservation{CheckID: 72, Status: "ok", IsFinal: true, ObservedAt: time.Unix(1800000000, 0), Payload: map[string]any{"cpu_percent": 10, "memory_percent": 20}}
	samples, err := ProjectSNMPObservation(job, observation, observation.ObservedAt, 60)
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range d.Items {
		if item.Key != samples.Samples[index].ItemKey || item.FreshnessSec != 60 {
			t.Fatal("definition diverged from legacy projection or trusted policy")
		}
	}
	job.Config["collector_id"] = 999
	job.IntervalSec = 3600
	relocated, err := DescribeSNMPItems(job, authority, policy)
	if err != nil || !reflect.DeepEqual(d, relocated) {
		t.Fatal("executor or legacy interval changed declared identity/policy")
	}
	delete(job.Config, "collector_id")
	job.IntervalSec = newSNMPJob().IntervalSec
	after, _ := json.Marshal(job)
	if string(before) != string(after) {
		t.Fatal("description mutated legacy job")
	}
}

func TestSNMPDefinitionRejectsScopeAndPolicyInsteadOfGuessing(t *testing.T) {
	authority := monitoring.DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{72}}
	for _, name := range []string{"client", "asset", "check", "source", "revision", "freshness", "retention"} {
		t.Run(name, func(t *testing.T) {
			job := newSNMPJob()
			job.CheckID = 72
			job.ClienteID = 3
			job.AtivoID = 52
			policy := SNMPDefinitionPolicy{Revision: 1, FreshnessSec: 60, HistoryDays: 30, TrendsDays: 365}
			switch name {
			case "client":
				job.ClienteID = 4
			case "asset":
				job.AtivoID = 53
			case "check":
				job.CheckID = 99
			case "source":
				job.Tipo = "http"
			case "revision":
				policy.Revision = 0
			case "freshness":
				policy.FreshnessSec = 0
			case "retention":
				policy.TrendsDays = 1
			}
			if _, err := DescribeSNMPItems(job, authority, policy); err == nil {
				t.Fatal("accepted incompatible job or absent trusted policy")
			}
		})
	}
}

func TestSNMPDefinitionDoesNotPromoteDiagnosticProbeToRoutineItems(t *testing.T) {
	job := newSNMPJob()
	job.CheckID = 72
	job.ClienteID = 3
	job.AtivoID = 52
	authority := monitoring.DefinitionAuthority{ClientID: 3, AssetID: 52, CheckIDs: []int64{72}}
	policy := SNMPDefinitionPolicy{Revision: 1, FreshnessSec: 60, HistoryDays: 30, TrendsDays: 365}
	for _, marker := range []any{true, "true", "false", 1, nil} {
		job.Config["diagnostic_probe"] = marker
		if _, err := DescribeSNMPItems(job, authority, policy); err == nil {
			t.Fatal("diagnostic/ambiguous marker promoted to routine definition")
		}
	}
	job.Config["diagnostic_probe"] = false
	if _, err := DescribeSNMPItems(job, authority, policy); err != nil {
		t.Fatal(err)
	}
}
