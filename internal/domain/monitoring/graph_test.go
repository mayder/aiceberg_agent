package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type graphFixture struct {
	Configuration Configuration          `json:"configuration"`
	Authority     ConfigurationAuthority `json:"configuration_authority"`
	Batch         RecordsBatch           `json:"batch"`
	Budget        PipelineBudget         `json:"budget"`
	Now           int64                  `json:"now_ms"`
	Pipelines     []struct {
		Wire      string            `json:"wire"`
		Authority PipelineAuthority `json:"authority"`
	} `json:"pipelines"`
}

func loadGraphFixture(t *testing.T) (graphFixture, []CompiledPipeline) {
	return loadGraphDefinition(t, "testdata/monitoring-graph-v1.json")
}

func loadGraphDefinition(t *testing.T, path string) (graphFixture, []CompiledPipeline) {
	t.Helper()
	var fixture graphFixture
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &fixture) != nil {
		t.Fatal("invalid graph fixture", err)
	}
	plans := make([]CompiledPipeline, 0, len(fixture.Pipelines))
	for _, input := range fixture.Pipelines {
		plan, err := CompilePipeline([]byte(input.Wire), input.Authority)
		if err != nil {
			t.Fatal(err)
		}
		plans = append(plans, plan)
	}
	return fixture, plans
}

func runGraphFixture(t *testing.T, f graphFixture, plans []CompiledPipeline) GraphExecution {
	t.Helper()
	g, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeGraphInput(g, f.Batch)
	if err != nil {
		t.Fatal(err)
	}
	out, err := RunConfiguredGraph(context.Background(), g, f.Budget, raw, f.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGraphFanoutUsesOneSourceAndIsolatesFailedSibling(t *testing.T) {
	f, plans := loadGraphFixture(t)
	out := runGraphFixture(t, f, plans)
	if out.Collected != 1 || out.Derived != 5 || out.Operations != 3 || out.Batch == nil ||
		out.Batch.Coverage != "complete" || len(out.Batch.Records) != 6 || len(out.Failures) != 1 {
		t.Fatal("incorrect graph execution", out)
	}
	want := map[string][3]string{
		"latency_ms": {"12", "ok", "none"},
		"echo":       {"12", "ok", "none"},
		"scaled":     {"12000", "ok", "none"},
		"offset":     {"12004", "ok", "none"},
		"range":      {"null", "unknown", "invalid"},
		"blocked":    {"null", "unknown", "dependency_unavailable"},
	}
	for _, record := range out.Batch.Records {
		metric := record.ItemKey[strings.LastIndex(record.ItemKey, ":")+1:]
		if want[metric] != [3]string{string(record.Value), record.State, record.Reason} || record.ObservedAtMS != f.Now {
			t.Fatal("wrong dependent record", metric, record)
		}
	}
	replayed := runGraphFixture(t, f, plans)
	if !reflect.DeepEqual(out, replayed) {
		t.Fatal("graph replay changed")
	}
	verifyGraphPHP(t, f, out)
}

func verifyGraphPHP(t *testing.T, f graphFixture, out GraphExecution) {
	t.Helper()
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		return
	}
	for script, data := range map[string]any{
		"monitoring-configuration.php": []map[string]any{{"configuration": f.Configuration, "authority": f.Authority}},
		"monitoring-records.php":       []map[string]any{{"batch": out.Batch, "definition": f.Configuration.Definition, "authority": f.Authority.Definition, "now_ms": f.Now}},
	} {
		wire, _ := json.Marshal(data)
		cmd := exec.Command("php", filepath.Join(root, "common/tests/_support", script))
		cmd.Stdin = bytes.NewReader(wire)
		result, err := cmd.CombinedOutput()
		var verdict []struct {
			Valid bool `json:"valid"`
		}
		if err != nil || json.Unmarshal(result, &verdict) != nil || len(verdict) != 1 || !verdict[0].Valid {
			t.Fatal("PHP rejected graph", script, err)
		}
	}
}

func TestGraphPreservesSourceQualityAndUsesDependentFreshness(t *testing.T) {
	f, plans := loadGraphFixture(t)
	f.Batch.Records[0].ObservedAtMS -= 15000
	f.Batch.Records[0].State = "problem"
	out := runGraphFixture(t, f, plans)
	for _, record := range out.Batch.Records {
		key := record.ItemKey
		if strings.HasSuffix(key, ":echo") && (record.State != "stale" || string(record.Value) != "null") {
			t.Fatal("dependent TTL was ignored")
		}
		if strings.HasSuffix(key, ":offset") && (record.State != "problem" || record.ObservedAtMS != f.Now-15000) {
			t.Fatal("source quality or clock was lost")
		}
	}
	f.Batch.Records[0].State = "unknown"
	f.Batch.Records[0].Reason = "observation_failed"
	f.Batch.Records[0].Value = json.RawMessage("null")
	out = runGraphFixture(t, f, plans)
	if out.Operations != 0 || out.Batch.Records[0].Reason != "observation_failed" {
		t.Fatal("unavailable source was processed")
	}
	for _, record := range out.Batch.Records[1:] {
		if string(record.Value) != "null" || !definitionEnum(record.Reason, "dependency_unavailable", "stale") {
			t.Fatal("unavailable source produced a value")
		}
	}
}

func TestGraphRejectsBadBindingsCyclesAndMissingFormulas(t *testing.T) {
	f, plans := loadGraphFixture(t)
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans[:2]); err == nil || err.Error() != "monitoring_graph_pipeline_missing" {
		t.Fatal("missing plan accepted", err)
	}
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, append(plans, plans[0])); err == nil {
		t.Fatal("duplicate plan accepted")
	}
	item := &f.Configuration.Definition.Items[0] // blocked depends on range.
	original := item.DependsOn[0]
	item.DependsOn[0] = item.Key
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans); err == nil || err.Error() != "monitoring_dependency_cycle" {
		t.Fatal("cycle accepted", err)
	}
	item.DependsOn[0] = original
	item.Unit = "percent"
	f.Configuration.Items[0].Parameters["unit"] = templateValueForGraph("text", "percent")
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans); err == nil || err.Error() != "monitoring_graph_type_mismatch" {
		t.Fatal("unit mismatch accepted", err)
	}
	item.Unit = "milliseconds"
	f.Configuration.Items[0].Parameters["unit"] = templateValueForGraph("text", "milliseconds")
	item.Kind = "calculated"
	f.Configuration.Items[0].Parameters["kind"] = templateValueForGraph("text", "calculated")
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans); err == nil || err.Error() != "invalid_monitoring_formula" {
		t.Fatal("calculated item without formula accepted", err)
	}
}

func templateValueForGraph(kind string, value any) ConfigurationValue {
	wire, _ := json.Marshal(value)
	return ConfigurationValue{Type: kind, Value: wire}
}

func TestGraphRejectsRestagingAndExhaustionWithoutPartialOutput(t *testing.T) {
	f, plans := loadGraphFixture(t)
	g, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeGraphInput(g, f.Batch)
	if err != nil {
		t.Fatal(err)
	}
	full := runGraphFixture(t, f, plans)
	final, _ := json.Marshal(full.Batch)
	if _, err := RunConfiguredGraph(context.Background(), g, f.Budget, final, f.Now, nil); err == nil {
		t.Fatal("final batch accepted as input")
	}
	f.Budget.MaxOperations = 2
	out, err := RunConfiguredGraph(context.Background(), g, f.Budget, raw, f.Now, nil)
	if err == nil || err.Error() != "pipeline_operation_limit" || out.Batch != nil {
		t.Fatal("partial graph output escaped", err)
	}
	f.Batch.Records = append(f.Batch.Records, f.Batch.Records[0])
	f.Batch.Records[1].ObservedAtMS--
	if _, err := EncodeGraphInput(g, f.Batch); err == nil || err.Error() != "monitoring_graph_duplicate_source" {
		t.Fatal("multiple source samples accepted", err)
	}
}

func TestGraphMissingMasterDoesNotInventAValueOrNewCollection(t *testing.T) {
	f, plans := loadGraphFixture(t)
	second := f.Configuration.Definition.Items[len(f.Configuration.Definition.Items)-1]
	second.Key, second.Metric = "v1:c:3:a:52:check:73:tcp:independent", "independent"
	f.Configuration.Definition.Items = append(f.Configuration.Definition.Items, second)
	item := ConfigurationItem{TemplateKey: "independent", ItemKey: second.Key, SourceItemID: 30,
		SourceTemplateID: 1, SourceRevision: 1, Parameters: map[string]ConfigurationValue{}}
	for k, v := range f.Configuration.Items[len(f.Configuration.Items)-1].Parameters {
		item.Parameters[k] = v
	}
	delete(item.Parameters, "configuration_ref")
	item.Parameters["metric"] = templateValueForGraph("text", second.Metric)
	f.Configuration.Items = append(f.Configuration.Items, item)
	f.Authority.Bindings[item.TemplateKey] = ConfigurationBinding{CheckID: 73, SourceItemID: 30, SourceTemplateID: 1}
	f.Batch.Records[0].ItemKey = second.Key
	f.Batch.Coverage = "partial"
	out := runGraphFixture(t, f, plans)
	if out.Collected != 1 || out.Derived != 5 || out.Operations != 0 || out.Batch.Coverage != "partial" {
		t.Fatal("missing master created another collection or a complete batch")
	}
	for _, record := range out.Batch.Records {
		if record.ItemKey != second.Key && (record.Reason != "dependency_unavailable" || string(record.Value) != "null") {
			t.Fatal("missing master produced a value")
		}
	}
}
