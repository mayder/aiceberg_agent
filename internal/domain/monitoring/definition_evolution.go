package monitoring

import "errors"

// ValidateDefinitionEvolution prevents a type/unit mutation of a persisted series.
// It applies neither configuration nor migration, storage, retention or alerts.
func ValidateDefinitionEvolution(previous, next Definition, a DefinitionAuthority) error {
	if err := ValidateDefinition(previous, a); err != nil {
		return err
	}
	if err := ValidateDefinition(next, a); err != nil {
		return err
	}
	return validateDefinitionSeriesEvolution(previous, next)
}

func validateDefinitionSeriesEvolution(previous, next Definition) error {
	if next.Revision <= previous.Revision {
		return errors.New("monitoring_revision_not_advanced")
	}
	old := map[string]ItemDefinition{}
	for _, item := range previous.Items {
		old[item.Key] = item
	}
	for _, item := range next.Items {
		if prior, ok := old[item.Key]; ok && (prior.ValueType != item.ValueType || prior.Unit != item.Unit) {
			return errors.New("monitoring_series_incompatible")
		}
	}
	return nil
}
