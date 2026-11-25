package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"strconv"
)

// Numeric operations start from bounded IEEE values; no unbounded exponent parsing.
// Rational arithmetic detects integer precision loss before any rounding can hide it.
func pipelineTransform(step PipelineStep, compiled compiledPipelineStep, raw json.RawMessage) (json.RawMessage, error) {
	if !validRecordValue(step.InputType, raw) {
		return nil, errors.New("pipeline_value_invalid")
	}
	if step.Operation == "identity" {
		return append([]byte{}, raw...), nil
	}
	switch step.Operation {
	case "validate", "regex", "jsonpath", "xmlpath":
		return pipelinePreprocess(step, compiled, raw)
	case "scale", "offset":
		return pipelineArithmetic(step, compiled.operand, raw)
	default:
		return nil, errors.New("unsupported_pipeline_step")
	}
}

func pipelineArithmetic(step PipelineStep, operand float64, raw json.RawMessage) (json.RawMessage, error) {
	value, err := strconv.ParseFloat(string(bytes.TrimSpace(raw)), 64)
	if err != nil {
		return nil, errors.New("pipeline_numeric_range")
	}
	left := new(big.Rat).SetFloat64(value)
	right := new(big.Rat).SetFloat64(operand)
	result := new(big.Rat)
	if step.Operation == "scale" {
		result.Mul(left, right)
	} else {
		result.Add(left, right)
	}
	return pipelineNumericResult(step.OutputType, result)
}

func pipelineNumericResult(kind string, result *big.Rat) (json.RawMessage, error) {
	if kind == "integer" {
		if !result.IsInt() || !result.Num().IsInt64() {
			return nil, errors.New("pipeline_numeric_range")
		}
		n := result.Num().Int64()
		if !definitionInteger(n, -9007199254740991, 9007199254740991) {
			return nil, errors.New("pipeline_numeric_range")
		}
		return json.Marshal(n)
	}
	n, _ := result.Float64()
	if math.IsInf(n, 0) || math.IsNaN(n) || (n == 0 && result.Sign() != 0) {
		return nil, errors.New("pipeline_numeric_range")
	}
	return json.Marshal(n)
}
