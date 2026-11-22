package monitoring

import "errors"

func pipelineInputDefinition(c Configuration, a ConfigurationAuthority, p CompiledPipeline) (Definition, map[string]bool, error) {
	if _, err := EncodeConfiguration(c, a); err != nil {
		return Definition{}, nil, err
	}
	pin := p.authority
	spec := p.definition
	if spec.Contract != "monitoring.pipeline.v1" || pin.SHA256 == "" || pin.ClientID != c.Definition.ClientID || pin.AssetID != c.Definition.AssetID || pin.ConfigurationRevision != c.Definition.Revision {
		return Definition{}, nil, errors.New("pipeline_scope_mismatch")
	}
	definitions := map[string]ItemDefinition{}
	items := map[string]ConfigurationItem{}
	for _, item := range c.Definition.Items {
		definitions[item.Key] = item
	}
	for _, item := range c.Items {
		items[item.ItemKey] = item
	}
	assigned := map[string]bool{}
	for _, key := range pin.ItemKeys {
		d, exists := definitions[key]
		item, found := items[key]
		if !exists || !found || !d.Enabled || configurationString(item.Parameters["pipeline_ref"]) != spec.ID || d.ValueType != spec.OutputType || d.Unit != spec.OutputUnit {
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
			result.Items[n].ValueType = spec.InputType
			result.Items[n].Unit = spec.InputUnit
		}
	}
	return result, assigned, nil
}

func validatePipelineBudget(b PipelineBudget) error {
	if b.MaxSteps < 1 || b.MaxSteps > MaxPipelineSteps || b.MaxOperations < 1 || b.MaxOperations > MaxRecords*MaxPipelineSteps || b.MaxInputBytes < 1 || b.MaxInputBytes > MaxRecordBytes || b.MaxOutputBytes < 1 || b.MaxOutputBytes > MaxRecordBytes || b.TimeoutMS < 1 || b.TimeoutMS > 2000 {
		return errors.New("invalid_pipeline_budget")
	}
	return nil
}
