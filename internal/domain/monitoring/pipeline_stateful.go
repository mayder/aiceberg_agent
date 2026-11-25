package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
)

// PipelineStateStore atomically commits one checkpoint per bound scope.
// nil from update is a read-only replay. Errors abort the transaction.
type PipelineStateStore interface {
	UpdatePipelineState(ctx context.Context, scope string, update func(previous []byte) ([]byte, error)) error
}

// StatefulPipelineExecution exposes suppression separately from observation quality.
// Result is nil when the entire input was suppressed.
type StatefulPipelineExecution struct {
	Result     *PipelineExecution `json:"result"`
	Suppressed int                `json:"suppressed"`
	Operations int                `json:"operations"`
}

func RunConfiguredStatefulPipeline(ctx context.Context, c Configuration, a ConfigurationAuthority, p CompiledPipeline, b PipelineBudget, raw []byte, now int64, store PipelineStateStore) (StatefulPipelineExecution, error) {
	if store == nil || !p.stateful {
		return StatefulPipelineExecution{}, errPipelineStateRequired
	}
	ctx, cancel, err := pipelineExecutionContext(ctx, b)
	if err != nil {
		return StatefulPipelineExecution{}, err
	}
	defer cancel()
	input, assigned, err := pipelinePrepareInput(c, a, p, b, raw, now)
	if err != nil {
		return StatefulPipelineExecution{}, err
	}
	scope, digest, err := pipelineStateIdentity(c, p.authority, input)
	if err != nil {
		return StatefulPipelineExecution{}, err
	}
	run := pipelineStateRun{ctx, p, b, input, assigned, c.Definition, a.Definition, now}
	var result StatefulPipelineExecution
	err = store.UpdatePipelineState(ctx, scope, func(previous []byte) ([]byte, error) {
		next, out, err := run.apply(previous, digest)
		result = out
		return next, err
	})
	if err != nil {
		return StatefulPipelineExecution{}, err
	}
	return result, nil
}

type pipelineStateRun struct {
	ctx        context.Context
	p          CompiledPipeline
	b          PipelineBudget
	input      RecordsBatch
	assigned   map[string]bool
	definition Definition
	authority  DefinitionAuthority
	now        int64
}

func (r pipelineStateRun) apply(previous []byte, digest string) ([]byte, StatefulPipelineExecution, error) {
	empty := StatefulPipelineExecution{}
	if err := pipelineContextError(r.ctx); err != nil {
		return nil, empty, err
	}
	state, err := decodePipelineCheckpoint(previous, r.assigned, len(r.p.steps))
	if err != nil {
		return nil, empty, err
	}
	if receipt := state.lookup(digest); receipt != nil {
		result, err := r.replay(receipt)
		return nil, result, err
	}
	batch, suppressed, err := pipelineRun(r.ctx, r.p, r.b, r.input, r.assigned, &state)
	if err != nil {
		return nil, empty, err
	}
	out := StatefulPipelineExecution{Suppressed: suppressed, Operations: batch.Operations}
	if len(batch.Batch.Records) != 0 {
		valid, err := pipelineFinish(r.ctx, batch, r.definition, r.authority, r.b, r.now)
		if err != nil {
			return nil, empty, err
		}
		out.Result = &valid
	}
	wire, err := json.Marshal(out)
	if err != nil || len(wire) > r.b.MaxOutputBytes {
		return nil, empty, errors.New("pipeline_output_limit")
	}
	next, err := state.appendReceipt(digest, wire)
	if err != nil {
		return nil, empty, err
	}
	return next, out, pipelineContextError(r.ctx)
}

func (r pipelineStateRun) replay(raw []byte) (StatefulPipelineExecution, error) {
	var result StatefulPipelineExecution
	if len(raw) > r.b.MaxOutputBytes {
		return result, errors.New("pipeline_output_limit")
	}
	if json.Unmarshal(raw, &result) != nil || result.Suppressed < 0 || result.Suppressed > len(r.input.Records) ||
		(result.Result == nil && result.Suppressed != len(r.input.Records)) {
		return StatefulPipelineExecution{}, errors.New("pipeline_checkpoint_invalid")
	}
	if result.Operations > r.b.MaxOperations {
		return StatefulPipelineExecution{}, errors.New("pipeline_operation_limit")
	}
	if result.Result != nil {
		if _, err := pipelineFinish(r.ctx, *result.Result, r.definition, r.authority, r.b, r.now); err != nil {
			return StatefulPipelineExecution{}, err
		}
	}
	return result, pipelineContextError(r.ctx)
}

func pipelineStateIdentity(c Configuration, a PipelineAuthority, batch RecordsBatch) (string, string, error) {
	a.ItemKeys = slices.Clone(a.ItemKeys)
	slices.Sort(a.ItemKeys)
	binding, err := json.Marshal(struct {
		Configuration Configuration
		Authority     PipelineAuthority
	}{c, a})
	if err != nil {
		return "", "", errors.New("pipeline_checkpoint_invalid")
	}
	input, err := json.Marshal(batch)
	if err != nil {
		return "", "", errors.New("invalid_pipeline_input")
	}
	scopeHash, inputHash := sha256.Sum256(binding), sha256.Sum256(input)
	return hex.EncodeToString(scopeHash[:]), hex.EncodeToString(inputHash[:]), nil
}
