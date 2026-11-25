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

type pipelineCase struct {
	Name                   string                 `json:"name"`
	Configuration          Configuration          `json:"configuration"`
	ConfigurationAuthority ConfigurationAuthority `json:"configuration_authority"`
	Wire                   string                 `json:"pipeline_wire"`
	Authority              PipelineAuthority      `json:"pipeline_authority"`
	Budget                 PipelineBudget         `json:"budget"`
	Batch                  RecordsBatch           `json:"batch"`
	Now                    int64                  `json:"now_ms"`
	CompileValid           bool                   `json:"compile_valid"`
	RunValid               bool                   `json:"run_valid"`
	Expected               struct {
		Value       json.RawMessage `json:"value"`
		State       string          `json:"state"`
		Reason      string          `json:"reason"`
		FailureCode string          `json:"failure_code"`
		Failed      int             `json:"failed"`
		Skipped     int             `json:"skipped"`
		Operations  int             `json:"operations"`
	} `json:"expected"`
}

var pipelineFixtureNames = []string{"pipeline-v1.json", "pipeline-xmlpath-v1.json"}

func pipelineCases(t testing.TB) []pipelineCase {
	t.Helper()
	var all []pipelineCase
	for _, name := range pipelineFixtureNames {
		raw, err := os.ReadFile(filepath.Join("testdata/", name))
		if err != nil {
			t.Fatal(err)
		}
		var cases []pipelineCase
		if err := json.Unmarshal(raw, &cases); err != nil || len(cases) == 0 {
			t.Fatalf("invalid fixture %s: %v", name, err)
		}
		all = append(all, cases...)
	}
	return all
}

func TestPipelineSharedVersionTypesMathStatesAndLimits(t *testing.T) {
	for _, c := range pipelineCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			before, _ := json.Marshal(c)
			p, err := CompilePipeline([]byte(c.Wire), c.Authority)
			if (err == nil) != c.CompileValid {
				t.Fatalf("compile mismatch: %v", err)
			}
			if err != nil {
				return
			}
			raw, _ := EncodePipelineInput(c.Batch, p.authority)
			result, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
			if (err == nil) != c.RunValid {
				t.Fatalf("execution mismatch: %v", err)
			}
			if err == nil {
				assertPipelineResult(t, c, result)
			} else if strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("payload leaked")
			}
			after, _ := json.Marshal(c)
			if !bytes.Equal(before, after) {
				t.Fatal("execution mutated input")
			}
		})
	}
}

func assertPipelineResult(t *testing.T, c pipelineCase, r PipelineExecution) {
	t.Helper()
	if r.Contract != "monitoring.pipeline-result.v1" || r.SchemaVersion != 1 || r.PipelineSHA256 != c.Authority.SHA256 || len(r.Batch.Records) != 1 || r.Failed != c.Expected.Failed || r.Skipped != c.Expected.Skipped || r.Operations != c.Expected.Operations {
		t.Fatal("invalid result counters", r)
	}
	got := r.Batch.Records[0]
	var left, right any
	if json.Unmarshal(got.Value, &left) != nil || json.Unmarshal(c.Expected.Value, &right) != nil || !reflect.DeepEqual(left, right) {
		t.Fatalf("wrong typed result: %s", got.Value)
	}
	if got.State != c.Expected.State || got.Reason != c.Expected.Reason || got.ItemKey != c.Batch.Records[0].ItemKey || got.ObservedAtMS != c.Batch.Records[0].ObservedAtMS || got.ValueType != c.Configuration.Definition.Items[0].ValueType || got.Unit != c.Configuration.Definition.Items[0].Unit || r.Batch.EvaluatedAtMS != c.Batch.EvaluatedAtMS {
		t.Fatal("identity/type/quality changed")
	}
	if len(r.Failures) != r.Failed {
		t.Fatal("missing safe failure trace")
	}
	code := c.Expected.FailureCode
	if code == "" {
		code = "pipeline_numeric_range"
	}
	for _, failure := range r.Failures {
		if failure.Code != code || failure.ItemKey != got.ItemKey || failure.ObservedAtMS != got.ObservedAtMS || failure.StepID != "step1" {
			t.Fatal("unsafe trace", failure)
		}
	}
}

func TestPipelinePHPValidationAndExecutedRecordConsumption(t *testing.T) {
	root := os.Getenv("AICEBERG_MONITORING_WEB_ROOT")
	if root == "" {
		t.Skip("explicit Web checkout required")
	}
	cases := pipelineCases(t)
	requests := []map[string]any{}
	for _, c := range cases {
		request := map[string]any{"pipeline_wire": c.Wire, "pipeline_authority": c.Authority}
		p, err := CompilePipeline([]byte(c.Wire), c.Authority)
		if err == nil && c.RunValid {
			raw, _ := EncodePipelineInput(c.Batch, p.authority)
			result, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
			if err != nil {
				t.Fatal(err)
			}
			request["result"] = result
			request["batch"] = c.Batch
			request["configuration"] = c.Configuration
			request["configuration_authority"] = c.ConfigurationAuthority
			request["now_ms"] = c.Now
		}
		requests = append(requests, request)
	}
	comparePipelinePHP(t, root, requests, cases)
}

func comparePipelinePHP(t *testing.T, root string, requests []map[string]any, cases []pipelineCase) {
	t.Helper()
	raw, _ := json.Marshal(requests)
	cmd := exec.Command("php", filepath.Join(root, "common/tests/_support/monitoring-pipeline.php"))
	cmd.Stdin = bytes.NewReader(raw)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("PHP pipeline helper: %v %s", err, output)
	}
	var results []struct {
		Valid    bool `json:"valid"`
		Consumed bool `json:"consumed"`
	}
	if json.Unmarshal(output, &results) != nil || len(results) != len(cases) {
		t.Fatal("invalid PHP receipt")
	}
	for i, c := range cases {
		if results[i].Valid != c.CompileValid || results[i].Consumed != (c.CompileValid && c.RunValid) {
			t.Fatalf("PHP acceptance mismatch %s: %+v", c.Name, results[i])
		}
	}
	for _, name := range pipelineFixtureNames {
		local, err := os.ReadFile(filepath.Join("testdata/", name))
		if err != nil {
			t.Fatal(err)
		}
		peer, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures", "monitoring-"+name))
		if err != nil || !bytes.Equal(local, peer) {
			t.Fatalf("pipeline fixture %s drift: %v", name, err)
		}
	}
}
