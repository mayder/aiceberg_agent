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
	if p.stateful {
		return PipelineExecution{}, errPipelineStateRequired
	}
	ctx, cancel, err := pipelineExecutionContext(ctx, b)
	if err != nil {
		return PipelineExecution{}, err
	}
	defer cancel()
	input, assigned, err := pipelinePrepareInput(c, a, p, b, raw, now)
	if err != nil {
		return PipelineExecution{}, err
	}
	out, _, err := pipelineRun(ctx, p, b, input, assigned, nil)
	if err != nil {
		return PipelineExecution{}, err
	}
	return pipelineFinish(ctx, out, c.Definition, a.Definition, b, now)
}

func pipelineExecutionContext(ctx context.Context, b PipelineBudget) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, errors.New("invalid_pipeline_context")
	}
	if err := validatePipelineBudget(b); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(b.TimeoutMS)*time.Millisecond)
	if err := pipelineContextError(ctx); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

func pipelineRun(ctx context.Context, p CompiledPipeline, b PipelineBudget, input RecordsBatch, assigned map[string]bool, state *pipelineCheckpoint) (PipelineExecution, int, error) {
	out := PipelineExecution{Contract: "monitoring.pipeline-result.v1", SchemaVersion: 1, PipelineSHA256: p.authority.SHA256, Batch: input, PipelineID: p.definition.ID, PipelineRevision: p.definition.Revision, Failures: []PipelineFailure{}}
	out.Batch.Records = make([]TypedRecord, 0, len(input.Records))
	suppressed := 0
	for _, record := range input.Records {
		if !assigned[record.ItemKey] {
			return PipelineExecution{}, 0, errors.New("pipeline_binding_mismatch")
		}
		item, err := state.advance(record)
		if err != nil {
			return PipelineExecution{}, 0, err
		}
		err = pipelineProcessRecord(ctx, p, b, record, &out, item)
		if err == errPipelineSuppressed {
			suppressed++
		} else if err != nil {
			return PipelineExecution{}, 0, err
		}
	}
	if suppressed > 0 {
		out.Batch.Coverage = "partial"
	}
	return out, suppressed, nil
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

func pipelineProcessRecord(ctx context.Context, p CompiledPipeline, b PipelineBudget, r TypedRecord, out *PipelineExecution, item *pipelineItemState) error {
	result, err := pipelineEvaluateRecord(ctx, p, b, r, out, item)
	if err == nil {
		out.Batch.Records = append(out.Batch.Records, result)
	}
	return err
}

func pipelineEvaluateRecord(ctx context.Context, p CompiledPipeline, b PipelineBudget, r TypedRecord, out *PipelineExecution, item *pipelineItemState) (TypedRecord, error) {
	if err := pipelineContextError(ctx); err != nil {
		return TypedRecord{}, err
	}
	result := r
	result.ValueType = p.definition.OutputType
	result.Unit = p.definition.OutputUnit
	if !definitionEnum(r.State, "ok", "problem") {
		result.Value = json.RawMessage("null")
		out.Skipped++
		return result, nil
	}
	out.Processed++
	value, failure, err := pipelineExecuteValue(ctx, p, b, r, out, item)
	if err == errPipelineWarmup {
		result.State, result.Reason, result.Value = "insufficient_data", "insufficient_data", json.RawMessage("null")
		return result, nil
	}
	if err == errPipelineSuppressed {
		result.Value = value
		return result, err
	}
	if err != nil {
		return TypedRecord{}, err
	}
	if failure != nil {
		if item != nil {
			clear(item.Steps)
		}
		result.State = "unknown"
		result.Reason = "invalid"
		result.Value = json.RawMessage("null")
		out.Failed++
		out.Failures = append(out.Failures, *failure)
	} else {
		result.Value = value
	}
	return result, nil
}

func pipelineExecuteValue(ctx context.Context, p CompiledPipeline, b PipelineBudget, r TypedRecord, out *PipelineExecution, item *pipelineItemState) (json.RawMessage, *PipelineFailure, error) {
	value := append([]byte{}, r.Value...)
	for n, step := range p.definition.Steps {
		if err := pipelineContextError(ctx); err != nil {
			return nil, nil, err
		}
		cost := 1 + p.steps[n].script.Cost()
		if cost > b.MaxOperations-out.Operations {
			return nil, nil, errors.New("pipeline_operation_limit")
		}
		out.Operations += cost
		next, err := pipelineRecordStep(ctx, step, p.steps[n], value, item, r, n)
		if err == errPipelineWarmup || err == errPipelineSuppressed {
			return value, nil, err
		}
		if fatal := pipelineScriptAbort(ctx, err); fatal != nil {
			return nil, nil, fatal
		}
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

func pipelineRecordStep(ctx context.Context, step PipelineStep, cfg compiledPipelineStep, raw json.RawMessage, item *pipelineItemState, r TypedRecord, n int) (json.RawMessage, error) {
	if step.Operation == "script" {
		return pipelineScriptValue(ctx, step, cfg, raw)
	}
	if !definitionEnum(step.Operation, "delta", "throttle") {
		return pipelineTransform(step, cfg, raw)
	}
	if item == nil {
		return nil, errPipelineStateRequired
	}
	state := item.Steps[n]
	var result json.RawMessage
	var err error
	if step.Operation == "delta" {
		result, err = pipelineDelta(step, cfg, raw, &state, r.ObservedAtMS)
	} else {
		result, err = pipelineThrottle(step, cfg, raw, &state, r)
	}
	item.Steps[n] = state
	if err == errPipelineWarmup {
		for index := range item.Steps {
			if index > n {
				delete(item.Steps, index)
			}
		}
	}
	return result, err
}
