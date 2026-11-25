package monitoring

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/you/aiceberg_agent/internal/domain/monitoring/internal/scriptsandbox"
)

const pipelineScriptABI = "wasm-json-v1"

func compilePipelineScript(step PipelineStep) (compiledPipelineStep, error) {
	args, ok := configurationObject(step.Argument, "abi", "module")
	var abi, module string
	if !ok || json.Unmarshal(args["abi"], &abi) != nil || abi != pipelineScriptABI ||
		json.Unmarshal(args["module"], &module) != nil || len(module) > base64.StdEncoding.EncodedLen(scriptsandbox.MaxModuleBytes) {
		return compiledPipelineStep{}, scriptsandbox.ErrInvalid
	}
	wire, err := base64.StdEncoding.Strict().DecodeString(module)
	if err != nil || base64.StdEncoding.EncodeToString(wire) != module {
		return compiledPipelineStep{}, scriptsandbox.ErrInvalid
	}
	program, err := scriptsandbox.Compile(wire)
	if err != nil {
		return compiledPipelineStep{}, err
	}
	return compiledPipelineStep{script: program}, nil
}

func pipelineScriptValue(ctx context.Context, step PipelineStep, cfg compiledPipelineStep, raw json.RawMessage) (json.RawMessage, error) {
	invalid := errors.New("pipeline_script_value_invalid")
	if !validRecordValue(step.InputType, raw) || (step.InputType == "integer" && !pipelineIntegerLiteral(raw)) {
		return nil, invalid
	}
	var input bytes.Buffer
	if err := json.Compact(&input, raw); err != nil {
		return nil, invalid
	}
	output, err := cfg.script.Run(ctx, input.Bytes())
	if err != nil {
		return nil, err
	}
	if !validRecordValue(step.OutputType, output) || (step.OutputType == "integer" && !pipelineIntegerLiteral(output)) {
		return nil, invalid
	}
	return json.RawMessage(bytes.TrimSpace(output)), nil
}

func pipelineScriptAbort(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return pipelineContextError(ctx)
	}
	if err == scriptsandbox.ErrBusy || err == scriptsandbox.ErrTimeout || err == scriptsandbox.ErrInvalid {
		return err
	}
	return nil
}
