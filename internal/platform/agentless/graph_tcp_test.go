package agentless

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/entities"
	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

func qualifyTCPMonitoringGraph(t *testing.T, observation entities.AgentlessObservation) {
	t.Helper()
	qualifyLatencyMonitoringGraph(t, observation, "tcp")
}

func qualifyLatencyMonitoringGraph(t *testing.T, observation entities.AgentlessObservation, source string) {
	t.Helper()
	raw, err := os.ReadFile("../../domain/monitoring/testdata/monitoring-graph-v1.json")
	var f struct {
		Configuration monitoring.Configuration          `json:"configuration"`
		Authority     monitoring.ConfigurationAuthority `json:"configuration_authority"`
		Budget        monitoring.PipelineBudget         `json:"budget"`
		Pipelines     []struct {
			Wire      string                       `json:"wire"`
			Authority monitoring.PipelineAuthority `json:"authority"`
		} `json:"pipelines"`
	}
	raw = []byte(strings.NewReplacer(`"tcp"`, `"`+source+`"`, `:tcp:`, ":"+source+":",
		`\"tcp\"`, `\"`+source+`\"`).Replace(string(raw)))
	if err != nil || json.Unmarshal(raw, &f) != nil {
		t.Fatal("invalid graph fixture", err)
	}
	var plans []monitoring.CompiledPipeline
	for _, artifact := range f.Pipelines {
		sum := sha256.Sum256([]byte(artifact.Wire))
		artifact.Authority.SHA256 = hex.EncodeToString(sum[:])
		p, err := monitoring.CompilePipeline([]byte(artifact.Wire), artifact.Authority)
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, p)
	}
	g, err := monitoring.CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	now := observation.ObservedAt.UnixMilli()
	staged, err := monitoring.EncodeProjectedGraphInput(g, []monitoring.Batch{*observation.MonitoringSamples}, now)
	if err != nil {
		t.Fatal(err)
	}
	out, err := monitoring.RunConfiguredGraph(context.Background(), g, f.Budget, staged, now, nil)
	checkTCPGraphFanout(t, out, err, observation.LatencyMs)
}

func checkTCPGraphFanout(t *testing.T, out monitoring.GraphExecution, err error, latency int) {
	t.Helper()
	if err != nil || out.Collected != 1 || out.Derived != 5 || out.Batch == nil {
		t.Fatal("one TCP collection did not reach its dependents", err)
	}
	values := map[string]string{}
	for _, record := range out.Batch.Records {
		values[record.ItemKey[strings.LastIndex(record.ItemKey, ":")+1:]] = string(record.Value)
	}
	if values["scaled"] != stringMustMarshal(latency*1000) || values["offset"] != stringMustMarshal(latency*1000+4) ||
		values["echo"] != stringMustMarshal(latency) {
		t.Fatal("wrong TCP fan-out values", values)
	}
}
