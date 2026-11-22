package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func pipelineRehash(raw []byte, a PipelineAuthority) PipelineAuthority {
	sum := sha256.Sum256(raw)
	a.SHA256 = hex.EncodeToString(sum[:])
	return a
}

func TestPipelineCancellationNeverReturnsPartialOutput(t *testing.T) {
	c := pipelineCases(t)[0]
	p, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := EncodePipelineInput(c.Batch, p.authority)
	expired, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	for _, ctx := range []context.Context{nil, expired, cancelled} {
		got, err := RunConfiguredPipeline(ctx, c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
		if err == nil || !reflect.DeepEqual(got, PipelineExecution{}) {
			t.Fatal("partial output escaped", err)
		}
	}
	if _, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, CompiledPipeline{}, c.Budget, raw, c.Now); err == nil {
		t.Fatal("zero plan accepted")
	}
}

func TestPipelineExact32Steps128Records4096OperationsAndBudgets(t *testing.T) {
	c := pipelineCases(t)[0]
	var definition PipelineDefinition
	if json.Unmarshal([]byte(c.Wire), &definition) != nil {
		t.Fatal("fixture")
	}
	first := definition.Steps[0]
	for n := 1; n < MaxPipelineSteps; n++ {
		next := first
		next.ID = "step" + strconv.Itoa(n)
		definition.Steps = append(definition.Steps, next)
	}
	// Use valid ASCII identifiers through all 32 positions.
	for n := range definition.Steps {
		definition.Steps[n].ID = "step" + strconv.Itoa(n)
	}
	wire, _ := json.Marshal(definition)
	p, err := CompilePipeline(wire, pipelineRehash(wire, c.Authority))
	if err != nil {
		t.Fatal(err)
	}
	record := c.Batch.Records[0]
	for n := 1; n < 128; n++ {
		next := record
		next.ObservedAtMS -= int64(n)
		c.Batch.Records = append(c.Batch.Records, next)
	}
	raw, _ := EncodePipelineInput(c.Batch, p.authority)
	out, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
	if err != nil || out.Operations != 4096 || len(out.Batch.Records) != 128 {
		t.Fatal("exact maximum rejected", err)
	}
	for _, budget := range []PipelineBudget{{32, 4095, 262144, 262144, 2000}, {32, 4096, len(raw) - 1, 262144, 2000}, {32, 4096, 262144, 1, 2000}} {
		result, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, budget, raw, c.Now)
		if err == nil || !reflect.DeepEqual(result, PipelineExecution{}) {
			t.Fatal("limit emitted partial output", err)
		}
	}
}

func TestPipelineWireLimitsAmbiguityAndOwnedCompiledPlan(t *testing.T) {
	c := pipelineCases(t)[0]
	raw := []byte(c.Wire)
	for _, bad := range [][]byte{[]byte("null"), []byte("[]"), []byte("{"), append(append([]byte{}, raw...), []byte("{}")...), bytes.Replace(raw, []byte(`"schema_version":1`), []byte(`"schema_version":1,"schema_version":1`), 1), bytes.Repeat([]byte(" "), MaxPipelineBytes+1)} {
		if _, err := CompilePipeline(bad, pipelineRehash(bad, c.Authority)); err == nil {
			t.Fatal("ambiguous artifact accepted")
		}
	}
	padded := append(append([]byte{}, raw...), bytes.Repeat([]byte(" "), MaxPipelineBytes-len(raw))...)
	if _, err := CompilePipeline(padded, pipelineRehash(padded, c.Authority)); err != nil {
		t.Fatal(err)
	}
	p, err := CompilePipeline(raw, c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '['
	c.Authority.ItemKeys[0] = "poisoned"
	input, _ := EncodePipelineInput(c.Batch, p.authority)
	if _, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, input, c.Now); err != nil {
		t.Fatal("caller poisoned immutable plan", err)
	}
}

func TestPipelineCompiledPlanIsConcurrentAndReplayStable(t *testing.T) {
	c := pipelineCases(t)[9]
	p, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := EncodePipelineInput(c.Batch, p.authority)
	expected, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for n := 0; n < 20; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			got, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
			if err != nil || !reflect.DeepEqual(got, expected) {
				t.Errorf("replay or shared state changed: %v", err)
			}
		}()
	}
	group.Wait()
}

func TestPipelineFinalResultsCannotBeFedBackAsRawInput(t *testing.T) {
	c := pipelineCases(t)[9]
	p, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodePipelineInput(c.Batch, p.authority)
	if err != nil {
		t.Fatal(err)
	}
	out, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	batch, _ := json.Marshal(out.Batch)
	result, _ := json.Marshal(out)
	for _, wire := range [][]byte{batch, result, bytes.Replace(raw, []byte(`"pipeline_revision":1`), []byte(`"pipeline_revision":2`), 1)} {
		got, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, wire, c.Now)
		if err == nil || !reflect.DeepEqual(got, PipelineExecution{}) {
			t.Fatal("stage replay accepted", err)
		}
	}
	recompiled, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, recompiled, c.Budget, raw, c.Now+86400000)
	if err != nil || !reflect.DeepEqual(replay, out) {
		t.Fatal("restart changed historical result", err)
	}
}

func TestPipelineInputEncoderRejectsInvalidAuthorityAndUnboundedWire(t *testing.T) {
	c := pipelineCases(t)[0]
	a := c.Authority
	a.SHA256 = "invalid"
	if _, err := EncodePipelineInput(c.Batch, a); err == nil {
		t.Fatal("invalid pin accepted")
	}
	c.Batch.Records[0].Value = json.RawMessage("{")
	if _, err := EncodePipelineInput(c.Batch, c.Authority); err == nil {
		t.Fatal("malformed value encoded")
	}
	c.Batch.Records[0].Value = json.RawMessage(`"` + string(bytes.Repeat([]byte("x"), MaxRecordBytes)) + `"`)
	if _, err := EncodePipelineInput(c.Batch, c.Authority); err == nil {
		t.Fatal("unbounded input encoded")
	}
}

func TestPipelineCancellationAtCommitBoundaryEmitsNoOutput(t *testing.T) {
	c := pipelineCases(t)[0]
	p, err := CompilePipeline([]byte(c.Wire), c.Authority)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := EncodePipelineInput(c.Batch, c.Authority)
	out, err := RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, raw, c.Now)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := pipelineFinish(ctx, out, c.Configuration.Definition, c.ConfigurationAuthority.Definition, c.Budget, c.Now)
	if err == nil || !reflect.DeepEqual(got, PipelineExecution{}) {
		t.Fatal("canceled commit emitted output")
	}
	out.Batch.Records[0].Value = json.RawMessage("null")
	got, err = pipelineFinish(context.Background(), out, c.Configuration.Definition, c.ConfigurationAuthority.Definition, c.Budget, c.Now)
	if err == nil || !reflect.DeepEqual(got, PipelineExecution{}) {
		t.Fatal("invalid final record emitted")
	}
}
