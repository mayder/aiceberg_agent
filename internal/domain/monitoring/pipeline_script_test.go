package monitoring

import (
	"context"
	"encoding/json"
	"testing"
)

func scriptCase(t *testing.T, name string) pipelineCase {
	t.Helper()
	for _, c := range pipelineCases(t) {
		if c.Name == name {
			return c
		}
	}
	t.Fatal("script fixture missing", name)
	return pipelineCase{}
}

func TestScriptInstructionsChargePipelineBudgetBeforeExecution(t *testing.T) {
	c := scriptCase(t, "sandbox-increment-ascii-digit")
	p, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodePipelineInput(c.Batch, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	budget := c.Budget
	budget.MaxOperations = c.Expected.Operations - 1
	ctx := context.Background()
	out, err := RunConfiguredPipeline(ctx, c.Configuration, c.ConfigurationAuthority, p, budget, raw, c.Now)
	if err == nil || err.Error() != "pipeline_operation_limit" || out.Batch.Records != nil {
		t.Fatal("script bypassed operation budget or returned partial output", err)
	}
	budget.MaxOperations++
	out, err = RunConfiguredPipeline(ctx, c.Configuration, c.ConfigurationAuthority, p, budget, raw, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	assertPipelineResult(t, c, out)
}

func TestScriptOutputFeedsGraphDependents(t *testing.T) {
	f, plans := loadGraphFixture(t)
	root := f.Batch.Records[0].ItemKey
	for _, item := range f.Configuration.Items {
		if item.ItemKey == root {
			item.Parameters["pipeline_ref"] = templateValueForGraph("text", "pipeline1")
		}
	}
	f.Authority.PipelineReferences = append(f.Authority.PipelineReferences, "pipeline1")
	f.Batch.Records[0].Value = json.RawMessage("4")
	plan := scriptGraphPlan(t, root)
	out := runGraphFixture(t, f, append(plans, plan))
	if out.Collected != 1 || out.Derived != 5 || out.Operations != 12 ||
		string(formulaRecord(t, out, "scaled").Value) != "5000" ||
		string(formulaRecord(t, out, "offset").Value) != "5004" {
		t.Fatal("script output did not feed dependents", out)
	}
	verifyGraphPHP(t, f, out)
}

func scriptGraphPlan(t *testing.T, key string) CompiledPipeline {
	t.Helper()
	c := scriptCase(t, "sandbox-increment-ascii-digit")
	var spec PipelineDefinition
	if err := json.Unmarshal([]byte(c.Wire), &spec); err != nil {
		t.Fatal(err)
	}
	spec.InputUnit, spec.OutputUnit = "milliseconds", "milliseconds"
	spec.Steps[0].InputUnit, spec.Steps[0].OutputUnit = spec.InputUnit, spec.OutputUnit
	wire, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	c.Authority.ItemKeys = []string{key}
	p, err := CompilePipeline(wire, pipelineRehash(wire, c.Authority))
	if err != nil {
		t.Fatal(err)
	}
	return p
}
