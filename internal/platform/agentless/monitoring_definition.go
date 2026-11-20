package agentless

import (
	"errors"
	"fmt"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

// SNMPDefinitionPolicy must come from the trusted configuration caller, not job.Config.
type SNMPDefinitionPolicy struct {
	Revision     int64
	FreshnessSec int64
	HistoryDays  int64
	TrendsDays   int64
}

// DescribeSNMPItems describes the existing two-series compatibility projection.
// It neither probes the device nor copies its endpoint/profile or changes the job.
// The caller resolves authority; a valid definition is not a capability/activation grant.
func DescribeSNMPItems(job entities.AgentlessJob, authority monitoring.DefinitionAuthority, policy SNMPDefinitionPolicy) (monitoring.Definition, error) {
	if job.Tipo != "snmp" {
		return monitoring.Definition{}, errors.New("invalid_monitoring_definition_source")
	}
	if marker, exists := job.Config["diagnostic_probe"]; exists && marker != false {
		return monitoring.Definition{}, errors.New("invalid_monitoring_definition_source")
	}
	d := monitoring.Definition{SchemaVersion: 1, Revision: policy.Revision, ClientID: int64(job.ClienteID), AssetID: int64(job.AtivoID), Items: make([]monitoring.ItemDefinition, 0, 2)}
	for _, metric := range []string{"cpu_percent", "memory_percent"} {
		d.Items = append(d.Items, monitoring.ItemDefinition{
			Key:     fmt.Sprintf("v1:c:%d:a:%d:check:%d:snmp:%s", job.ClienteID, job.AtivoID, job.CheckID, metric),
			CheckID: int64(job.CheckID), Metric: metric, Source: "snmp", Kind: "collected", ValueType: "number", Unit: "percent",
			FreshnessSec: policy.FreshnessSec, HistoryDays: policy.HistoryDays, TrendsDays: policy.TrendsDays,
			Enabled: true, DependsOn: []string{},
		})
	}
	if err := monitoring.ValidateDefinition(d, authority); err != nil {
		return monitoring.Definition{}, err
	}
	return d, nil
}
