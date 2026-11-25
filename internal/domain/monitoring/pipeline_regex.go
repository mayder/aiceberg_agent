package monitoring

import (
	"encoding/json"
	"errors"
	"regexp"
	"regexp/syntax"
	"strconv"
)

const maxPipelineRegexCost = 1024

func compilePipelineRegex(step PipelineStep) (compiledPipelineStep, error) {
	invalid := errors.New("invalid_pipeline_step")
	args, ok := configurationObject(step.Argument, "pattern", "group")
	if !ok || step.InputType != "text" || step.OutputType != "text" || step.InputUnit != step.OutputUnit {
		return compiledPipelineStep{}, invalid
	}
	var pattern string
	group, err := strconv.Atoi(string(args["group"]))
	if err != nil || group < 0 || group > 16 || json.Unmarshal(args["pattern"], &pattern) != nil ||
		len(pattern) == 0 || !recordText(pattern, 256) {
		return compiledPipelineStep{}, invalid
	}
	tree, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil || pipelineRegexCost(tree) > maxPipelineRegexCost {
		return compiledPipelineStep{}, invalid
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil || compiled.NumSubexp() > 16 || group > compiled.NumSubexp() {
		return compiledPipelineStep{}, invalid
	}
	return compiledPipelineStep{pattern: compiled, group: group}, nil
}

// Bound expansion before regexp.Compile builds the program. Counted repetition
// multiplies the cost of the subtree, including nested repetitions.
func pipelineRegexCost(tree *syntax.Regexp) int {
	cost := 3 + len(tree.Rune)
	for _, child := range tree.Sub {
		cost += pipelineRegexCost(child)
		if cost > maxPipelineRegexCost {
			return maxPipelineRegexCost + 1
		}
	}
	if tree.Op == syntax.OpRepeat {
		copies := tree.Max
		if copies < 0 {
			copies = tree.Min + 1
		}
		cost *= copies + 1
	}
	return cost
}

func pipelineRegexExtract(compiled compiledPipelineStep, input string) (json.RawMessage, error) {
	match := compiled.pattern.FindStringSubmatchIndex(input)
	index := 2 * compiled.group
	if match == nil || match[index] < 0 {
		return nil, errors.New("pipeline_regex_no_match")
	}
	return json.Marshal(input[match[index]:match[index+1]])
}
