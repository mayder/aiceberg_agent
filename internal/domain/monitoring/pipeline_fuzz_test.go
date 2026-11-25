package monitoring

import (
	"context"
	"testing"
)

func FuzzPipelineBoundedCompileAndExecutionNeverPanic(f *testing.F) {
	cases := pipelineCases(f)
	c := cases[0]
	f.Add([]byte(c.Wire))
	for _, item := range cases {
		if item.CompileValid {
			f.Add([]byte(item.Wire))
		}
	}
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
