package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

type graphNode struct {
	item     ItemDefinition
	pipeline CompiledPipeline
	formula  []formulaInstruction
}

// CompiledGraph owns validated DAG bindings and takes observations from a single
// collection snapshot. It has no collector or access to source credentials.
type CompiledGraph struct {
	nodes      []graphNode
	definition Definition
	input      Definition
	authority  DefinitionAuthority
	digest     string
	stateful   bool
}

func CompileMonitoringGraph(c Configuration, a ConfigurationAuthority, plans []CompiledPipeline) (CompiledGraph, error) {
	raw, err := EncodeConfiguration(c, a)
	if err != nil {
		return CompiledGraph{}, err
	}
	if len(plans) > MaxDefinitionItems {
		return CompiledGraph{}, errors.New("monitoring_graph_pipeline_limit")
	}
	c, err = DecodeConfiguration(raw, a)
	if err != nil {
		return CompiledGraph{}, err
	}
	bindings, err := graphPipelines(c, plans)
	if err != nil {
		return CompiledGraph{}, err
	}
	g, err := graphBuild(c, bindings)
	if err != nil {
		return CompiledGraph{}, err
	}
	g.authority = a.Definition
	g.authority.CheckIDs = append([]int64(nil), a.Definition.CheckIDs...)
	g.digest = graphDigest(raw, g.nodes)
	return g, nil
}

func graphPipelines(c Configuration, plans []CompiledPipeline) (map[string]CompiledPipeline, error) {
	definitions, items := pipelineConfigurationItems(c)
	bindings := make(map[string]CompiledPipeline)
	for _, p := range plans {
		if err := pipelineMatchesScope(c, p); err != nil {
			return nil, err
		}
		for _, key := range p.authority.ItemKeys {
			d, exists := definitions[key]
			item, found := items[key]
			if !exists || !found || !pipelineMatchesItem(d, item, p) || bindings[key].definition.ID != "" {
				return nil, errors.New("pipeline_binding_mismatch")
			}
			bindings[key] = p
		}
	}
	for key, d := range definitions {
		if d.Enabled && configurationString(items[key].Parameters["pipeline_ref"]) != "" && bindings[key].definition.ID == "" {
			return nil, errors.New("monitoring_graph_pipeline_missing")
		}
	}
	return bindings, nil
}

func graphBuild(c Configuration, bindings map[string]CompiledPipeline) (CompiledGraph, error) {
	d := c.Definition
	g := CompiledGraph{definition: d, input: d}
	g.input.Items = nil
	items, declared := pipelineConfigurationItems(c)
	depths, marks := map[string]int{}, map[string]int{}
	for _, item := range d.Items {
		if !item.Enabled {
			continue
		}
		p := bindings[item.Key]
		input := graphInputItem(item, p)
		node, err := graphCompileNode(item, input, p, items, declared[item.Key])
		if err != nil {
			return CompiledGraph{}, err
		}
		if item.Kind == "collected" {
			g.input.Items = append(g.input.Items, input)
		}
		if _, err := definitionDepth(item.Key, items, marks, depths); err != nil {
			return CompiledGraph{}, err
		}
		g.nodes = append(g.nodes, node)
		g.stateful = g.stateful || p.stateful
	}
	if len(g.input.Items) == 0 {
		return CompiledGraph{}, errors.New("monitoring_graph_no_collectors")
	}
	sort.Slice(g.nodes, func(i, j int) bool {
		left, right := g.nodes[i].item.Key, g.nodes[j].item.Key
		if depths[left] != depths[right] {
			return depths[left] < depths[right]
		}
		return left < right
	})
	return g, nil
}

func graphInputItem(item ItemDefinition, p CompiledPipeline) ItemDefinition {
	if p.definition.ID != "" {
		item.ValueType, item.Unit = p.definition.InputType, p.definition.InputUnit
		if !definitionEnum(item.ValueType, "number", "integer") {
			item.TrendsDays = 0
		}
	}
	return item
}

func graphCompileNode(item, input ItemDefinition, p CompiledPipeline, items map[string]ItemDefinition, declared ConfigurationItem) (graphNode, error) {
	node := graphNode{item: item, pipeline: p}
	switch item.Kind {
	case "calculated":
		formula, err := compileFormula(configurationString(declared.Parameters["formula"]), input, items)
		if err != nil {
			return graphNode{}, err
		}
		node.formula = formula
	case "dependent":
		parent := items[item.DependsOn[0]]
		if parent.ValueType != input.ValueType || parent.Unit != input.Unit {
			return graphNode{}, errors.New("monitoring_graph_type_mismatch")
		}
	}
	return node, nil
}

func graphDigest(configuration []byte, nodes []graphNode) string {
	pins := make([]string, len(nodes))
	for i, node := range nodes {
		pins[i] = node.pipeline.authority.SHA256
	}
	wire, _ := json.Marshal(pins)
	digest := sha256.New()
	_, _ = digest.Write(configuration)
	_, _ = digest.Write(wire)
	return hex.EncodeToString(digest.Sum(nil))
}
