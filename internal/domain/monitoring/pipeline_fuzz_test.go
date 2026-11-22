package monitoring

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func FuzzPipelineBoundedCompileAndExecutionNeverPanic(f *testing.F) {
	raw, err := os.ReadFile("testdata/pipeline-v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var cases []pipelineCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) == 0 {
		f.Fatal("invalid fixture")
	}
	c := cases[0]
	f.Add([]byte(c.Wire))
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))

	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > MaxPipelineBytes {
			return
		}
		p, err := CompilePipeline(wire, pipelineRehash(wire, c.Authority))
		if err != nil {
			return
		}
		input, err := EncodePipelineInput(c.Batch, p.authority)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = RunConfiguredPipeline(context.Background(), c.Configuration, c.ConfigurationAuthority, p, c.Budget, input, c.Now)
	})
}
