package monitoring

import (
	"context"
	"encoding/json"
	"errors"
)

// GraphExecution is a local receipt. Only Batch, when present, is transported.
type GraphExecution struct {
	GraphSHA256 string            `json:"graph_sha256"`
	Batch       *RecordsBatch     `json:"batch"`
	Collected   int               `json:"collected"`
	Derived     int               `json:"derived"`
	Suppressed  int               `json:"suppressed"`
	Operations  int               `json:"operations"`
	Failures    []PipelineFailure `json:"failures"`
}

// graphValue also tracks the oldest source through calculated nodes. Their
// result timestamp belongs to the evaluation cycle, not to a single operand.
type graphValue struct {
	record   TypedRecord
	oldestMS int64
}

type graphRun struct {
	ctx    context.Context
	graph  CompiledGraph
	budget PipelineBudget
	input  RecordsBatch
	now    int64
}

// RunConfiguredGraph processes a staged collection snapshot, once per graph node.
func RunConfiguredGraph(ctx context.Context, g CompiledGraph, b PipelineBudget, raw []byte, now int64, store PipelineStateStore) (GraphExecution, error) {
	if g.stateful && store == nil {
		return GraphExecution{}, errPipelineStateRequired
	}
	ctx, cancel, err := pipelineExecutionContext(ctx, b)
	if err != nil {
		return GraphExecution{}, err
	}
	defer cancel()
	batch, err := graphDecodeInput(g, b, raw, now)
	if err != nil {
		return GraphExecution{}, err
	}
	for _, node := range g.nodes {
		if len(node.pipeline.steps) > b.MaxSteps || len(node.formula) > b.MaxSteps {
			return GraphExecution{}, errors.New("pipeline_step_limit")
		}
	}
	run := graphRun{ctx, g, b, batch, now}
	if g.stateful {
		return run.transaction(store)
	}
	return run.execute(nil)
}

func (r graphRun) execute(state *pipelineCheckpoint) (GraphExecution, error) {
	masters := make(map[string]graphValue, len(r.graph.nodes))
	sources := make(map[string]TypedRecord, len(r.input.Records))
	for _, record := range r.input.Records {
		sources[record.ItemKey] = record
	}
	receipt := GraphExecution{GraphSHA256: r.graph.digest, Collected: len(sources), Failures: []PipelineFailure{}}
	batch, counts := r.input, PipelineExecution{Failures: receipt.Failures}
	batch.Records = make([]TypedRecord, 0, len(r.graph.nodes))
	for _, node := range r.graph.nodes {
		if err := pipelineContextError(r.ctx); err != nil {
			return GraphExecution{}, err
		}
		input, exists, err := r.nodeInput(node, sources, masters, &counts)
		if err != nil {
			return GraphExecution{}, err
		}
		if !exists {
			continue
		}
		record, err := r.evaluate(node, input.record, state, &counts)
		if err != nil && err != errPipelineSuppressed {
			return GraphExecution{}, err
		}
		masters[node.item.Key] = graphValue{record: record, oldestMS: input.oldestMS}
		if err == errPipelineSuppressed {
			receipt.Suppressed++
			continue
		}
		batch.Records = append(batch.Records, record)
		if node.item.Kind != "collected" {
			receipt.Derived++
		}
	}
	receipt.Operations, receipt.Failures = counts.Operations, counts.Failures
	return r.finish(batch, receipt)
}

func (r graphRun) finish(batch RecordsBatch, receipt GraphExecution) (GraphExecution, error) {
	if len(batch.Records) != 0 {
		if len(batch.Records) != len(r.graph.nodes) {
			batch.Coverage = "partial"
		}
		receipt.Batch = &batch
	}
	if err := r.validateResult(receipt); err != nil {
		return GraphExecution{}, err
	}
	return receipt, nil
}

func (r graphRun) nodeInput(node graphNode, sources map[string]TypedRecord, masters map[string]graphValue, counts *PipelineExecution) (graphValue, bool, error) {
	if node.item.Kind == "collected" {
		record, exists := sources[node.item.Key]
		return graphValue{record, record.ObservedAtMS}, exists, nil
	}
	if node.item.Kind == "calculated" {
		record, err := r.calculatedInput(node, masters, counts)
		return record, true, err
	}
	master, exists := masters[node.item.DependsOn[0]]
	return graphDependentRecord(graphInputItem(node.item, node.pipeline), master, exists, r.input.EvaluatedAtMS), true, nil
}

func graphDependentRecord(item ItemDefinition, master graphValue, exists bool, at int64) graphValue {
	out := TypedRecord{ItemKey: item.Key, ValueType: item.ValueType, Unit: item.Unit, ObservedAtMS: at,
		State: "unknown", Reason: "dependency_unavailable", Value: json.RawMessage("null")}
	if !exists {
		return graphValue{out, at}
	}
	out.ObservedAtMS = master.record.ObservedAtMS
	freshness, _ := RecordFreshness(master.oldestMS, item.FreshnessSec, at)
	switch {
	case out.ObservedAtMS > at:
		out.Reason = "future"
	case freshness == "stale":
		out.ObservedAtMS, out.State, out.Reason = master.oldestMS, "stale", "stale"
	case freshness == "fresh":
		if definitionEnum(master.record.State, "ok", "problem") {
			out.State, out.Reason, out.Value = master.record.State, master.record.Reason, master.record.Value
		}
	}
	return graphValue{out, master.oldestMS}
}

func (r graphRun) evaluate(node graphNode, input TypedRecord, state *pipelineCheckpoint, counts *PipelineExecution) (TypedRecord, error) {
	if node.pipeline.definition.ID == "" {
		return input, nil
	}
	var item *pipelineItemState
	if node.pipeline.stateful {
		var err error
		item, err = state.advance(input)
		if err != nil {
			return TypedRecord{}, err
		}
	}
	return pipelineEvaluateRecord(r.ctx, node.pipeline, r.budget, input, counts, item)
}

func (r graphRun) validateResult(out GraphExecution) error {
	if out.GraphSHA256 != r.graph.digest {
		return errors.New("monitoring_graph_scope_mismatch")
	}
	if out.Operations < 0 || out.Operations > r.budget.MaxOperations {
		return errors.New("pipeline_operation_limit")
	}
	if out.Batch != nil {
		if err := ValidateRecords(*out.Batch, r.graph.definition, r.graph.authority, r.now); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > r.budget.MaxOutputBytes {
		return errors.New("pipeline_output_limit")
	}
	return pipelineContextError(r.ctx)
}
