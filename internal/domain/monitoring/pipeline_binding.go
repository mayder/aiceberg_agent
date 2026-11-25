package monitoring

import "errors"

func pipelineInputDefinition(c Configuration, a ConfigurationAuthority, p CompiledPipeline) (Definition, map[string]bool, error) {
	if _, err := EncodeConfiguration(c, a); err != nil {
		return Definition{}, nil, err
	}
	if err := pipelineMatchesScope(c, p); err != nil {
		return Definition{}, nil, err
	}
	definitions, items := pipelineConfigurationItems(c)
	assigned := map[string]bool{}
	for _, key := range p.authority.ItemKeys {
		d, exists := definitions[key]
		item, found := items[key]
		if !exists || !found || !pipelineMatchesItem(d, item, p) {
			return Definition{}, nil, errors.New("pipeline_binding_mismatch")
		}
		if d.Kind != "collected" {
			return Definition{}, nil, errors.New("unsupported_pipeline_binding")
		}
		assigned[key] = true
	}
	result := c.Definition
	result.Items = append([]ItemDefinition{}, c.Definition.Items...)
	for n, item := range result.Items {
		if assigned[item.Key] {
			result.Items[n] = graphInputItem(item, p)
		}
	}
	return result, assigned, nil
}

func pipelineMatchesScope(c Configuration, p CompiledPipeline) error {
	pin := p.authority
	spec := p.definition
	if spec.Contract != "monitoring.pipeline.v1" || pin.SHA256 == "" || pin.ClientID != c.Definition.ClientID || pin.AssetID != c.Definition.AssetID || pin.ConfigurationRevision != c.Definition.Revision {
		return errors.New("pipeline_scope_mismatch")
	}
	return nil
}

func pipelineConfigurationItems(c Configuration) (map[string]ItemDefinition, map[string]ConfigurationItem) {
	definitions := map[string]ItemDefinition{}
	items := map[string]ConfigurationItem{}
	for _, item := range c.Definition.Items {
		definitions[item.Key] = item
	}
	for _, item := range c.Items {
		items[item.ItemKey] = item
	}
	return definitions, items
}

func pipelineMatchesItem(d ItemDefinition, item ConfigurationItem, p CompiledPipeline) bool {
	return d.Enabled && configurationString(item.Parameters["pipeline_ref"]) == p.definition.ID &&
		d.ValueType == p.definition.OutputType && d.Unit == p.definition.OutputUnit
}

func validatePipelineBudget(b PipelineBudget) error {
	if b.MaxSteps < 1 || b.MaxSteps > MaxPipelineSteps || b.MaxOperations < 1 || b.MaxOperations > MaxRecords*MaxPipelineSteps || b.MaxInputBytes < 1 || b.MaxInputBytes > MaxRecordBytes || b.MaxOutputBytes < 1 || b.MaxOutputBytes > MaxRecordBytes || b.TimeoutMS < 1 || b.TimeoutMS > 2000 {
		return errors.New("invalid_pipeline_budget")
	}
	return nil
}
