package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// RunConfiguredPipeline consumes bounded raw records; it never collects or persists.
// Numeric step errors become explicit unknown/null; budget/config errors return no partial output.
func RunConfiguredPipeline(ctx context.Context, c Configuration, a ConfigurationAuthority, p CompiledPipeline, b PipelineBudget, raw []byte, now int64) (PipelineExecution, error) {
	if ctx == nil {
		return PipelineExecution{}, errors.New("invalid_pipeline_context")
	}
	if err := validatePipelineBudget(b); err != nil {
		return PipelineExecution{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(b.TimeoutMS)*time.Millisecond)
	defer cancel()
	if err := pipelineContextError(ctx); err != nil {
		return PipelineExecution{}, err
	}
	input, assigned, err := pipelinePrepareInput(c, a, p, b, raw, now)
	if err != nil {
		return PipelineExecution{}, err
	}
	out := PipelineExecution{Contract: "monitoring.pipeline-result.v1", SchemaVersion: 1, PipelineSHA256: p.authority.SHA256, Batch: input, PipelineID: p.definition.ID, PipelineRevision: p.definition.Revision, Failures: []PipelineFailure{}}
	out.Batch.Records = make([]TypedRecord, 0, len(input.Records))
	for _, record := range input.Records {
		if !assigned[record.ItemKey] {
			return PipelineExecution{}, errors.New("pipeline_binding_mismatch")
		}
		if err := pipelineProcessRecord(ctx, p, b, record, &out); err != nil {
			return PipelineExecution{}, err
		}
	}
	return pipelineFinish(ctx, out, c.Definition, a.Definition, b, now)
}

func pipelinePrepareInput(c Configuration, a ConfigurationAuthority, p CompiledPipeline, b PipelineBudget, raw []byte, now int64) (RecordsBatch, map[string]bool, error) {
	inputDefinition, assigned, err := pipelineInputDefinition(c, a, p)
	if err != nil {
		return RecordsBatch{}, nil, err
	}
	if len(p.definition.Steps) > b.MaxSteps {
		return RecordsBatch{}, nil, errors.New("pipeline_step_limit")
	}
	if len(raw) > b.MaxInputBytes {
		return RecordsBatch{}, nil, errors.New("pipeline_input_limit")
	}
	input, err := decodePipelineInput(raw, p)
	if err == nil {
		err = ValidateRecords(input, inputDefinition, a.Definition, now)
	}
	if err != nil {
		return RecordsBatch{}, nil, err
	}
	return input, assigned, nil
}

func pipelineFinish(ctx context.Context, out PipelineExecution, d Definition, a DefinitionAuthority, b PipelineBudget, now int64) (PipelineExecution, error) {
	if err := ValidateRecords(out.Batch, d, a, now); err != nil {
		return PipelineExecution{}, err
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > b.MaxOutputBytes {
		return PipelineExecution{}, errors.New("pipeline_output_limit")
	}
	if err := pipelineContextError(ctx); err != nil {
		return PipelineExecution{}, err
	}
	return out, nil
}

func pipelineProcessRecord(ctx context.Context, p CompiledPipeline, b PipelineBudget, r TypedRecord, out *PipelineExecution) error {
	if err := pipelineContextError(ctx); err != nil {
		return err
	}
	result := r
	result.ValueType = p.definition.OutputType
	result.Unit = p.definition.OutputUnit
	if !definitionEnum(r.State, "ok", "problem") {
		result.Value = json.RawMessage("null")
		out.Skipped++
		out.Batch.Records = append(out.Batch.Records, result)
		return nil
	}
	out.Processed++
	value, failure, err := pipelineExecuteValue(ctx, p, b, r, out)
	if err != nil {
		return err
	}
	if failure != nil {
		result.State = "unknown"
		result.Reason = "invalid"
		result.Value = json.RawMessage("null")
		out.Failed++
		out.Failures = append(out.Failures, *failure)
	} else {
		result.Value = value
	}
	out.Batch.Records = append(out.Batch.Records, result)
	return nil
}

func pipelineExecuteValue(ctx context.Context, p CompiledPipeline, b PipelineBudget, r TypedRecord, out *PipelineExecution) (json.RawMessage, *PipelineFailure, error) {
	value := append([]byte{}, r.Value...)
	for n, step := range p.definition.Steps {
		if err := pipelineContextError(ctx); err != nil {
			return nil, nil, err
		}
		if out.Operations >= b.MaxOperations {
			return nil, nil, errors.New("pipeline_operation_limit")
		}
		out.Operations++
		next, err := pipelineTransform(step, p.operands[n], value)
		if err != nil {
			return nil, &PipelineFailure{ObservedAtMS: r.ObservedAtMS, ItemKey: r.ItemKey, StepID: step.ID, Code: err.Error()}, nil
		}
		value = next
	}
	return value, nil, nil
}

func pipelineContextError(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return errors.New("pipeline_timeout")
	}
	if ctx.Err() != nil {
		return errors.New("pipeline_cancelled")
	}
	return nil
}
