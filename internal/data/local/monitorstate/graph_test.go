package monitorstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/you/aiceberg_agent/internal/domain/monitoring"
)

type durableGraphFixture struct {
	Configuration monitoring.Configuration          `json:"configuration"`
	Authority     monitoring.ConfigurationAuthority `json:"configuration_authority"`
	Batch         monitoring.RecordsBatch           `json:"batch"`
	Budget        monitoring.PipelineBudget         `json:"budget"`
	Now           int64                             `json:"now_ms"`
	Pipelines     []struct {
		Wire      string                       `json:"wire"`
		Authority monitoring.PipelineAuthority `json:"authority"`
	} `json:"pipelines"`
}

func loadDurableGraph(t *testing.T) (durableGraphFixture, monitoring.CompiledGraph) {
	t.Helper()
	var f durableGraphFixture
	raw, err := os.ReadFile("../../../domain/monitoring/testdata/monitoring-graph-v1.json")
	if err != nil || json.Unmarshal(raw, &f) != nil {
		t.Fatal("invalid graph fixture", err)
	}
	plans := make([]monitoring.CompiledPipeline, 0, len(f.Pipelines)+2)
	for _, p := range f.Pipelines {
		compiled, err := monitoring.CompilePipeline([]byte(p.Wire), p.Authority)
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, compiled)
	}
	for i, item := range f.Configuration.Definition.Items {
		if item.Metric == "echo" {
			f.Configuration.Definition.Items[i].Kind = "calculated"
		}
	}
	for _, item := range f.Configuration.Items {
		if item.TemplateKey == "echo" {
			item.Parameters["kind"] = monitoring.ConfigurationValue{Type: "text", Value: json.RawMessage(`"calculated"`)}
			item.Parameters["formula"] = monitoring.ConfigurationValue{Type: "text", Value: json.RawMessage(`"d1 i:one:2 *"`)}
		}
	}
	plans = append(plans, bindGraphTestPlan(t, &f, "latency_ms", "throttle", "30000"))
	plans = append(plans, bindGraphTestPlan(t, &f, "echo", "delta", `"gauge"`))
	g, err := monitoring.CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	return f, g
}

func bindGraphTestPlan(t *testing.T, f *durableGraphFixture, metric, operation, argument string) monitoring.CompiledPipeline {
	t.Helper()
	var key string
	for _, item := range f.Configuration.Definition.Items {
		if item.Metric == metric {
			key = item.Key
		}
	}
	id := "test-" + operation
	wireID, _ := json.Marshal(id)
	for _, item := range f.Configuration.Items {
		if item.ItemKey == key {
			item.Parameters["pipeline_ref"] = monitoring.ConfigurationValue{Type: "text", Value: wireID}
		}
	}
	f.Authority.PipelineReferences = append(f.Authority.PipelineReferences, id)
	step := monitoring.PipelineStep{ID: "step1", Operation: operation, InputType: "integer", OutputType: "integer",
		InputUnit: "milliseconds", OutputUnit: "milliseconds", Argument: json.RawMessage(argument)}
	spec := monitoring.PipelineDefinition{Contract: "monitoring.pipeline.v1", SchemaVersion: 1, ID: id, Revision: 1, ClientID: 3,
		InputType: step.InputType, OutputType: step.OutputType, InputUnit: step.InputUnit, OutputUnit: step.OutputUnit, Steps: []monitoring.PipelineStep{step}}
	if operation == "delta" {
		c := fixture(t, "sandbox-echo-integer")
		var artifact monitoring.PipelineDefinition
		if err := json.Unmarshal([]byte(c.Wire), &artifact); err != nil {
			t.Fatal(err)
		}
		script := artifact.Steps[0]
		script.ID, script.InputUnit, script.OutputUnit = "script1", "milliseconds", "milliseconds"
		spec.Steps = append([]monitoring.PipelineStep{script}, spec.Steps...)
	}
	wire, _ := json.Marshal(spec)
	sum := sha256.Sum256(wire)
	authority := monitoring.PipelineAuthority{ClientID: 3, AssetID: 52, ConfigurationRevision: 1,
		ID: id, Revision: 1, SHA256: hex.EncodeToString(sum[:]), ItemKeys: []string{key}}
	plan, err := monitoring.CompilePipeline(wire, authority)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestStatefulGraphRetainsSuppressedMasterAndRollsBackAsAUnit(t *testing.T) {
	f, g := loadDurableGraph(t)
	store, path := createState(t)
	run := func(s *BoltStore, budget monitoring.PipelineBudget) (monitoring.GraphExecution, error) {
		raw, err := monitoring.EncodeGraphInput(g, f.Batch)
		if err != nil {
			t.Fatal(err)
		}
		return monitoring.RunConfiguredGraph(context.Background(), g, budget, raw, f.Batch.EvaluatedAtMS, s)
	}
	first, err := run(store, f.Budget)
	if err != nil || graphResult(first, ":echo").State != "insufficient_data" {
		t.Fatal("dependent did not start a base", err)
	}
	f.Batch.EvaluatedAtMS++
	f.Batch.Records[0].ObservedAtMS++
	tight := f.Budget
	tight.MaxOperations = 2
	if bad, err := run(store, tight); err == nil || bad.Batch != nil {
		t.Fatal("budget returned partial output", err)
	}
	got, err := run(store, f.Budget)
	if err != nil || got.Suppressed != 1 || got.Derived != 5 || got.Batch.Coverage != "partial" ||
		string(graphResult(got, ":echo").Value) != "0" {
		t.Fatal("throttle blocked child or changed the base during rollback", got, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replayed, err := run(reopened, f.Budget)
	if err != nil || !reflect.DeepEqual(got, replayed) {
		t.Fatal("graph replay altered persisted output", err)
	}
}

func graphResult(out monitoring.GraphExecution, suffix string) monitoring.TypedRecord {
	if out.Batch != nil {
		for _, record := range out.Batch.Records {
			if strings.HasSuffix(record.ItemKey, suffix) {
				return record
			}
		}
	}
	return monitoring.TypedRecord{}
}
