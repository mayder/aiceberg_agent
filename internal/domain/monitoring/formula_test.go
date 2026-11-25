package monitoring

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func formulaFixture(t *testing.T) (graphFixture, []CompiledPipeline) {
	t.Helper()
	return loadGraphDefinition(t, "testdata/monitoring-formulas-v1.json")
}

func formulaRecord(t *testing.T, result GraphExecution, metric string) TypedRecord {
	t.Helper()
	if result.Batch != nil {
		for _, record := range result.Batch.Records {
			if strings.HasSuffix(record.ItemKey, ":"+metric) {
				return record
			}
		}
	}
	t.Fatal("record not found", metric)
	return TypedRecord{}
}

func TestGraphCalculatesTypedFormulasAndKeepsFailureLocal(t *testing.T) {
	f, plans := formulaFixture(t)
	f.Batch.Records[1].State = "problem"
	out := runGraphFixture(t, f, plans)
	if out.Collected != 2 || out.Derived != 11 || out.Operations != 26 || len(out.Failures) != 2 {
		t.Fatal("unexpected formula execution counts", out)
	}
	for metric, want := range map[string][3]string{
		"sum":        {"20", "integer", "milliseconds"},
		"mean":       {"10", "number", "milliseconds"},
		"ratio":      {"1.5", "number", "one"},
		"double_sum": {"40", "integer", "milliseconds"},
	} {
		got := formulaRecord(t, out, metric)
		if want != [3]string{string(got.Value), got.ValueType, got.Unit} || got.State != "problem" || got.ObservedAtMS != f.Now {
			t.Fatal("calculation changed type, clock or quality", metric, got)
		}
	}
	for metric, reason := range map[string]string{"divide_zero": "invalid", "after_failure": "dependency_unavailable"} {
		got := formulaRecord(t, out, metric)
		if got.State != "unknown" || got.Reason != reason || string(got.Value) != "null" {
			t.Fatal("invalid branch fabricated a value", metric, got)
		}
	}
	found := false
	for _, failure := range out.Failures {
		found = found || (failure.StepID == "formula" && failure.Code == "formula_division_by_zero")
	}
	if !found {
		t.Fatal("division by zero was not diagnosed")
	}
	verifyGraphPHP(t, f, out)
}

func TestCalculatedItemsRespectMissingStaleAndFutureOperands(t *testing.T) {
	for _, mode := range []string{"missing", "stale", "future"} {
		t.Run(mode, func(t *testing.T) {
			f, plans := formulaFixture(t)
			var state, reason string
			switch mode {
			case "missing":
				f.Batch.Records = f.Batch.Records[:1]
				f.Batch.Coverage = "partial"
				state, reason = "unknown", "dependency_unavailable"
			case "stale":
				r := &f.Batch.Records[1]
				r.ObservedAtMS = f.Now - 61000
				r.State, r.Reason, r.Value = "stale", "stale", json.RawMessage("null")
				state, reason = "stale", "stale"
			case "future":
				r := &f.Batch.Records[1]
				r.ObservedAtMS = f.Now + 1
				r.State, r.Reason, r.Value = "unknown", "future", json.RawMessage("null")
				state, reason = "unknown", "future"
			}
			out := runGraphFixture(t, f, plans)
			if got := formulaRecord(t, out, "sum"); got.State != state || got.Reason != reason || string(got.Value) != "null" {
				t.Fatal("formula evaluated with unusable operands", mode, got)
			}
			if out.Operations != 3 {
				t.Fatal("formula charged operations for missing operands", mode)
			}
		})
	}
}

func setGraphFormula(t *testing.T, f *graphFixture, key, expression string) {
	t.Helper()
	for _, item := range f.Configuration.Items {
		if item.TemplateKey == key {
			item.Parameters["formula"] = templateValueForGraph("text", expression)
			return
		}
	}
	t.Fatal("unknown test item", key)
}

func TestFormulaCompileRejectsUnknownDependenciesTypesUnitsAndPrograms(t *testing.T) {
	for _, expression := range []string{
		"d1 d3 +", "d1", "d1 d2", "d1 +", "d1 d2 *", "d1 d2 /",
		"d1 number d2 +", "d1 d2 call", "d1 d2 + n:one:1e-999 *",
		"d1 d2 + i:one:9007199254740992 *", strings.Repeat("d1 ", 33),
	} {
		f, plans := formulaFixture(t)
		setGraphFormula(t, &f, "sum", expression)
		if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans); err == nil {
			t.Fatal("invalid formula compiled", expression)
		}
	}
	f, plans := formulaFixture(t)
	setGraphFormula(t, &f, "echo", "d1")
	if _, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans); err == nil {
		t.Fatal("formula on dependent item compiled")
	}
}

func TestFormulaIntegerDoesNotRoundAndRangeIsBounded(t *testing.T) {
	f, plans := formulaFixture(t)
	setGraphFormula(t, &f, "sum", "d1 d2 + i:one:3 /")
	out := runGraphFixture(t, f, plans)
	if got := formulaRecord(t, out, "sum"); got.Reason != "invalid" || string(got.Value) != "null" {
		t.Fatal("fractional integer was emitted", got)
	}
	setGraphFormula(t, &f, "sum", "d1 d2 + i:one:9007199254740991 *")
	out = runGraphFixture(t, f, plans)
	if got := formulaRecord(t, out, "sum"); got.Reason != "invalid" {
		t.Fatal("overflow accepted", got)
	}
	setGraphFormula(t, &f, "sum", "d1 d2 - neg abs d2 max d1 min")
	out = runGraphFixture(t, f, plans)
	if got := formulaRecord(t, out, "sum"); string(got.Value) != "8" {
		t.Fatal("wrong unary/min/max result", got)
	}
}

func TestFormulaBudgetAndBindingRejectPartialAndOldStage(t *testing.T) {
	f, plans := formulaFixture(t)
	g, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := EncodeGraphInput(g, f.Batch)
	if err != nil {
		t.Fatal(err)
	}
	tight := f.Budget
	tight.MaxSteps = 6
	if out, err := RunConfiguredGraph(context.Background(), g, tight, wire, f.Now, nil); err == nil || out.Batch != nil {
		t.Fatal("formula length ignored budget", err)
	}
	tight = f.Budget
	tight.MaxOperations = 5
	if out, err := RunConfiguredGraph(context.Background(), g, tight, wire, f.Now, nil); err == nil || out.Batch != nil {
		t.Fatal("formula operations returned partial batch", err)
	}
	setGraphFormula(t, &f, "sum", "d1 d2 -")
	changed, err := CompileMonitoringGraph(f.Configuration, f.Authority, plans)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RunConfiguredGraph(context.Background(), changed, f.Budget, wire, f.Now, nil); err == nil {
		t.Fatal("stage reused after formula change")
	}
}

func TestFormulaSeparatesResultIdentityFromOldestInputFreshness(t *testing.T) {
	f, plans := formulaFixture(t)
	for i, item := range f.Configuration.Definition.Items {
		if item.Metric == "double_sum" {
			f.Configuration.Definition.Items[i].FreshnessSec = 3
		}
	}
	for _, item := range f.Configuration.Items {
		if item.TemplateKey == "double_sum" {
			item.Parameters["freshness_sec"] = templateValueForGraph("integer", 3)
		}
	}
	first := runGraphFixture(t, f, plans)
	if got := formulaRecord(t, first, "double_sum"); string(got.Value) != "40" {
		t.Fatal("fresh operands did not reach the second calculation", got)
	}
	f.Now += 3000
	f.Batch.EvaluatedAtMS = f.Now
	f.Batch.Records[0].ObservedAtMS = f.Now
	f.Batch.Records[0].Value = json.RawMessage("14")
	second := runGraphFixture(t, f, plans)
	if got := formulaRecord(t, second, "sum"); string(got.Value) != "22" || got.ObservedAtMS != f.Now {
		t.Fatal("new calculation reused old record identity", got)
	}
	if got := formulaRecord(t, second, "double_sum"); got.State != "stale" || got.ObservedAtMS != f.Batch.Records[1].ObservedAtMS {
		t.Fatal("a fresh calculated timestamp renewed the old operand", got)
	}
	verifyGraphPHP(t, f, second)
}
