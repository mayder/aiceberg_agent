package monitoring

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPipelineRejectsUnsupportedOrUnboundedRegexBeforeExecution(t *testing.T) {
	for _, test := range []struct {
		pattern string
		group   int
	}{
		{pattern: "(?=a)"},
		{pattern: `(a)\1`},
		{pattern: "a{1000}"},
		{pattern: strings.Repeat("(a)", 17)},
		{pattern: "a", group: 1},
	} {
		c := pipelineCases(t)[3]
		var spec PipelineDefinition
		if err := json.Unmarshal([]byte(c.Wire), &spec); err != nil {
			t.Fatal(err)
		}
		step := &spec.Steps[0]
		step.Operation = "regex"
		step.Argument, _ = json.Marshal(map[string]any{"pattern": test.pattern, "group": test.group})
		wire, _ := json.Marshal(spec)
		if _, err := CompilePipeline(wire, pipelineRehash(wire, c.Authority)); err == nil {
			t.Errorf("accepted unsupported or unbounded pattern %q", test.pattern)
		}
	}
}
