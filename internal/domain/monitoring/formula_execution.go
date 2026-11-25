package monitoring

import (
	"encoding/json"
	"errors"
	"math/big"
)

func (r graphRun) calculatedInput(node graphNode, masters map[string]graphValue, counts *PipelineExecution) (graphValue, error) {
	input := graphInputItem(node.item, node.pipeline)
	raw, parents := graphFormulaParents(input, masters, r.input.EvaluatedAtMS)
	if !definitionEnum(raw.record.State, "ok", "problem") {
		return raw, nil
	}
	value, failure, fatal := r.runFormula(node.formula, parents, counts)
	if fatal != nil {
		return graphValue{}, fatal
	}
	if failure != nil {
		raw.record.State, raw.record.Reason, raw.record.Value = "unknown", "invalid", json.RawMessage("null")
		counts.Failures = append(counts.Failures, PipelineFailure{ObservedAtMS: raw.record.ObservedAtMS, ItemKey: raw.record.ItemKey, StepID: "formula", Code: failure.Error()})
		return raw, nil
	}
	raw.record.Value = value
	return raw, nil
}

func graphFormulaParents(item ItemDefinition, masters map[string]graphValue, evaluated int64) (graphValue, []TypedRecord) {
	out := TypedRecord{ItemKey: item.Key, ValueType: item.ValueType, Unit: item.Unit, ObservedAtMS: evaluated,
		State: "ok", Reason: "none", Value: json.RawMessage("null")}
	parents := make([]TypedRecord, 0, len(item.DependsOn))
	missing, future, oldest := false, int64(0), evaluated
	for _, key := range item.DependsOn {
		parent, exists := masters[key]
		if !exists {
			missing = true
			continue
		}
		parents = append(parents, parent.record)
		oldest = min(oldest, parent.oldestMS)
		future = max(future, parent.record.ObservedAtMS)
		if !definitionEnum(parent.record.State, "ok", "problem") {
			missing = true
		} else if parent.record.State == "problem" {
			out.State = "problem"
		}
	}
	switch {
	case future > evaluated:
		out.ObservedAtMS, out.State, out.Reason = future, "unknown", "future"
	case evaluated-oldest > item.FreshnessSec*1000:
		out.ObservedAtMS, out.State, out.Reason = oldest, "stale", "stale"
	case missing:
		out.State, out.Reason = "unknown", "dependency_unavailable"
	}
	return graphValue{out, oldest}, parents
}

func (r graphRun) runFormula(program []formulaInstruction, parents []TypedRecord, counts *PipelineExecution) (json.RawMessage, error, error) {
	stack := make([]float64, 0, len(program))
	var result json.RawMessage
	for _, inst := range program {
		if err := pipelineContextError(r.ctx); err != nil {
			return nil, nil, err
		}
		if counts.Operations >= r.budget.MaxOperations {
			return nil, nil, errors.New("pipeline_operation_limit")
		}
		counts.Operations++
		var err error
		result, stack, err = evalFormulaInstruction(inst, stack, parents)
		if err != nil {
			return nil, err, nil
		}
	}
	return result, nil, nil
}

func evalFormulaInstruction(inst formulaInstruction, stack []float64, parents []TypedRecord) (json.RawMessage, []float64, error) {
	var result *big.Rat
	switch inst.op {
	case "dep":
		raw := parents[inst.parent].Value
		if inst.result.kind == "integer" && !pipelineIntegerLiteral(raw) {
			return nil, nil, errors.New("formula_invalid_operand")
		}
		result = new(big.Rat).SetFloat64(pipelineNumber(raw))
	case "literal":
		result = new(big.Rat).SetFloat64(inst.literal)
	case "abs", "neg", "number":
		last := len(stack) - 1
		result = new(big.Rat).SetFloat64(stack[last])
		stack = stack[:last]
		switch inst.op {
		case "abs":
			result.Abs(result)
		case "neg":
			result.Neg(result)
		}
	default:
		last := len(stack) - 1
		left, right := new(big.Rat).SetFloat64(stack[last-1]), new(big.Rat).SetFloat64(stack[last])
		stack = stack[:last-1]
		var err error
		result, err = formulaBinary(inst.op, left, right)
		if err != nil {
			return nil, nil, err
		}
	}
	raw, err := pipelineNumericResult(inst.result.kind, result)
	if err != nil {
		return nil, nil, err
	}
	return raw, append(stack, pipelineNumber(raw)), nil
}

func formulaBinary(op string, left, right *big.Rat) (*big.Rat, error) {
	switch op {
	case "+":
		return left.Add(left, right), nil
	case "-":
		return left.Sub(left, right), nil
	case "*":
		return left.Mul(left, right), nil
	case "/":
		if right.Sign() == 0 {
			return nil, errors.New("formula_division_by_zero")
		}
		return left.Quo(left, right), nil
	case "min":
		if right.Cmp(left) < 0 {
			return right, nil
		}
		return left, nil
	case "max":
		if right.Cmp(left) > 0 {
			return right, nil
		}
		return left, nil
	}
	return nil, errInvalidFormula
}
