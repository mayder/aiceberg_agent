package monitoring

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const maxFormulaTokens = 32

var (
	formulaToken      = regexp.MustCompile(`^(?:d(?:[1-9]|1[0-6])|i:[a-z][a-z0-9_]{0,31}:-?(?:0|[1-9][0-9]*)|n:[a-z][a-z0-9_]{0,31}:-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?|[+*/-]|min|max|abs|neg|number)$`)
	errInvalidFormula = errors.New("invalid_monitoring_formula")
	errFormulaType    = errors.New("monitoring_formula_type_mismatch")
)

type formulaType struct {
	kind string
	unit string
}

type formulaInstruction struct {
	op      string
	result  formulaType
	parent  int
	literal float64
}

func formulaTokens(expression string) []string {
	if len(expression) == 0 || len(expression) > 256 || !recordText(expression, 256) {
		return nil
	}
	tokens := strings.Fields(expression)
	if len(tokens) == 0 || len(tokens) > maxFormulaTokens {
		return nil
	}
	// Only ASCII spaces separate tokens. Unicode spaces must not be normalized.
	if strings.Join(tokens, "") != strings.ReplaceAll(expression, " ", "") {
		return nil
	}
	for _, term := range tokens {
		if !formulaToken.MatchString(term) {
			return nil
		}
	}
	return tokens
}

type formulaCompiler struct {
	target  ItemDefinition
	items   map[string]ItemDefinition
	stack   []formulaType
	program []formulaInstruction
	used    map[int]bool
}

func compileFormula(expression string, target ItemDefinition, items map[string]ItemDefinition) ([]formulaInstruction, error) {
	tokens := formulaTokens(expression)
	if len(tokens) == 0 {
		return nil, errInvalidFormula
	}
	c := formulaCompiler{target: target, items: items, used: map[int]bool{}}
	for _, term := range tokens {
		inst, err := c.instruction(term)
		if err != nil {
			return nil, err
		}
		c.stack = append(c.stack, inst.result)
		c.program = append(c.program, inst)
	}
	if len(c.stack) != 1 || len(c.used) != len(target.DependsOn) {
		return nil, errInvalidFormula
	}
	if c.stack[0] != (formulaType{target.ValueType, target.Unit}) {
		return nil, errFormulaType
	}
	return c.program, nil
}

func (c *formulaCompiler) instruction(term string) (formulaInstruction, error) {
	if term[0] == 'd' {
		index, _ := strconv.Atoi(term[1:])
		if index < 1 || index > len(c.target.DependsOn) {
			return formulaInstruction{}, errInvalidFormula
		}
		item := c.items[c.target.DependsOn[index-1]]
		if !definitionEnum(item.ValueType, "integer", "number") {
			return formulaInstruction{}, errFormulaType
		}
		c.used[index-1] = true
		return formulaInstruction{op: "dep", parent: index - 1, result: formulaType{item.ValueType, item.Unit}}, nil
	}
	if strings.Contains(term, ":") {
		return formulaLiteral(term)
	}
	return c.operator(term)
}

func formulaLiteral(term string) (formulaInstruction, error) {
	fields := strings.Split(term, ":")
	kind := "number"
	if fields[0] == "i" {
		kind = "integer"
	}
	raw := json.RawMessage(fields[2])
	if !validRecordValue(kind, raw) || (kind == "integer" && !pipelineIntegerLiteral(raw)) {
		return formulaInstruction{}, errInvalidFormula
	}
	return formulaInstruction{op: "literal", literal: pipelineNumber(raw), result: formulaType{kind, fields[1]}}, nil
}

func (c *formulaCompiler) operator(op string) (formulaInstruction, error) {
	n := len(c.stack)
	if definitionEnum(op, "abs", "neg", "number") {
		if n == 0 {
			return formulaInstruction{}, errInvalidFormula
		}
		result := c.stack[n-1]
		c.stack = c.stack[:n-1]
		if op == "number" {
			result.kind = "number"
		}
		return formulaInstruction{op: op, result: result}, nil
	}
	if n < 2 {
		return formulaInstruction{}, errInvalidFormula
	}
	left, right := c.stack[n-2], c.stack[n-1]
	c.stack = c.stack[:n-2]
	result, err := formulaBinaryType(op, left, right)
	return formulaInstruction{op: op, result: result}, err
}

func formulaBinaryType(op string, left, right formulaType) (formulaType, error) {
	if left.kind != right.kind {
		return formulaType{}, errFormulaType
	}
	switch op {
	case "+", "-", "min", "max":
		if left.unit == right.unit {
			return left, nil
		}
	case "*":
		if left.unit == "one" {
			return right, nil
		}
		if right.unit == "one" {
			return left, nil
		}
	case "/":
		if left.unit == right.unit {
			return formulaType{left.kind, "one"}, nil
		}
		if right.unit == "one" {
			return left, nil
		}
	}
	return formulaType{}, errFormulaType
}
