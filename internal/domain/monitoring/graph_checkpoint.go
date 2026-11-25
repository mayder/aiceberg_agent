package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

func (r graphRun) transaction(store PipelineStateStore) (GraphExecution, error) {
	wire, err := json.Marshal(r.input)
	if err != nil {
		return GraphExecution{}, errors.New("invalid_monitoring_graph_input")
	}
	hash := sha256.Sum256(wire)
	key := hex.EncodeToString(hash[:])
	assigned := make(map[string]bool)
	for _, node := range r.graph.nodes {
		if node.pipeline.stateful {
			assigned[node.item.Key] = true
		}
	}
	var result GraphExecution
	err = store.UpdatePipelineState(r.ctx, r.graph.digest, func(previous []byte) ([]byte, error) {
		next, out, err := r.apply(previous, key, assigned)
		result = out
		return next, err
	})
	if err != nil {
		return GraphExecution{}, err
	}
	return result, nil
}

func (r graphRun) apply(previous []byte, key string, assigned map[string]bool) ([]byte, GraphExecution, error) {
	if err := pipelineContextError(r.ctx); err != nil {
		return nil, GraphExecution{}, err
	}
	state, err := decodePipelineCheckpoint(previous, assigned, MaxPipelineSteps)
	if err != nil {
		return nil, GraphExecution{}, err
	}
	if receipt := state.lookup(key); receipt != nil {
		var out GraphExecution
		if json.Unmarshal(receipt, &out) != nil {
			return nil, GraphExecution{}, errors.New("pipeline_checkpoint_invalid")
		}
		return nil, out, r.validateResult(out)
	}
	if r.input.EvaluatedAtMS <= state.EvaluationMS {
		return nil, GraphExecution{}, errors.New("monitoring_graph_snapshot_conflict")
	}
	state.EvaluationMS = r.input.EvaluatedAtMS
	out, err := r.execute(&state)
	if err != nil {
		return nil, GraphExecution{}, err
	}
	wire, err := json.Marshal(out)
	if err != nil {
		return nil, GraphExecution{}, errors.New("pipeline_checkpoint_invalid")
	}
	next, err := state.appendReceipt(key, wire)
	if err != nil {
		return nil, GraphExecution{}, err
	}
	return next, out, pipelineContextError(r.ctx)
}
