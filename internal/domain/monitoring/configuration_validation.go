package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// ValidateConfiguration rechecks compiled data against independently resolved authority.
func ValidateConfiguration(c Configuration, a ConfigurationAuthority) error {
	if c.Contract != "monitoring.configuration.v1" || c.SchemaVersion != 1 || len(c.Items) < 1 || len(c.Items) > MaxDefinitionItems {
		return errors.New("invalid_monitoring_configuration")
	}
	if err := ValidateDefinition(c.Definition, a.Definition); err != nil {
		return err
	}
	if err := validateConfigurationAuthority(c, a); err != nil {
		return err
	}
	if len(c.Items) != len(c.Definition.Items) || len(a.Bindings) != len(c.Items) {
		return errors.New("monitoring_configuration_binding_mismatch")
	}
	definitions := map[string]ItemDefinition{}
	for _, item := range c.Definition.Items {
		definitions[item.Key] = item
	}
	keys := map[string]string{}
	for _, item := range c.Items {
		if _, duplicate := keys[item.TemplateKey]; duplicate {
			return errors.New("monitoring_configuration_binding_mismatch")
		}
		keys[item.TemplateKey] = item.ItemKey
	}
	seen := map[string]bool{}
	for _, item := range c.Items {
		d, exists := definitions[item.ItemKey]
		if !exists || seen[item.ItemKey] {
			return errors.New("monitoring_configuration_binding_mismatch")
		}
		seen[item.ItemKey] = true
		if err := validateConfigurationItem(item, d, keys, a); err != nil {
			return err
		}
	}
	return nil
}

func validateConfigurationAuthority(c Configuration, a ConfigurationAuthority) error {
	if !definitionInteger(a.Revision, 1, 9007199254740991) || c.Definition.Revision != a.Revision || c.Root != a.Root || len(c.Lineage) < 1 || len(c.Lineage) > 64 || len(a.TemplateRevisions) != len(c.Lineage) || len(a.SecretReferences) > 128 || len(a.PipelineReferences) > 128 {
		return errors.New("monitoring_configuration_scope_mismatch")
	}
	if a.Root.ID < 1 || a.Root.Revision < 1 || a.TemplateRevisions[a.Root.ID] != a.Root.Revision {
		return errors.New("monitoring_configuration_scope_mismatch")
	}
	seen := map[int64]bool{}
	for _, pin := range c.Lineage {
		if !definitionInteger(pin.ID, 1, 9007199254740991) || !definitionInteger(pin.Revision, 1, 9007199254740991) || seen[pin.ID] || a.TemplateRevisions[pin.ID] != pin.Revision {
			return errors.New("monitoring_configuration_scope_mismatch")
		}
		seen[pin.ID] = true
	}
	if !configurationReferences(a.SecretReferences, configurationSecret) || !configurationReferences(a.PipelineReferences, configurationIdentifier) {
		return errors.New("invalid_monitoring_authority")
	}
	return nil
}

func validateConfigurationItem(i ConfigurationItem, d ItemDefinition, keys map[string]string, a ConfigurationAuthority) error {
	binding, exists := a.Bindings[i.TemplateKey]
	if !exists || !configurationIdentifier.MatchString(i.TemplateKey) || binding.CheckID != d.CheckID || binding.SourceItemID != i.SourceItemID || binding.SourceTemplateID != i.SourceTemplateID || !definitionInteger(i.SourceItemID, 1, 9007199254740991) || !definitionInteger(i.SourceTemplateID, 1, 9007199254740991) || !definitionInteger(i.SourceRevision, 1, 9007199254740991) || a.TemplateRevisions[i.SourceTemplateID] != i.SourceRevision {
		return errors.New("monitoring_configuration_binding_mismatch")
	}
	if err := validateConfigurationParameters(i.Parameters, a); err != nil {
		return err
	}
	p := i.Parameters
	if _, ok := p["formula"]; ok && d.Kind != "calculated" {
		return errInvalidFormula
	}
	for name, want := range map[string]string{"metric": d.Metric, "source": d.Source, "kind": d.Kind, "unit": d.Unit, "value_type": d.ValueType} {
		if configurationString(p[name]) != want {
			return errors.New("monitoring_configuration_definition_mismatch")
		}
	}
	for name, want := range map[string]int64{"freshness_sec": d.FreshnessSec, "history_days": d.HistoryDays, "trends_days": d.TrendsDays} {
		if configurationNumber(p[name]) != want {
			return errors.New("monitoring_configuration_definition_mismatch")
		}
	}
	var enabled bool
	_ = json.Unmarshal(p["enabled"].Value, &enabled)
	var deps []string
	_ = json.Unmarshal(p["depends_on"].Value, &deps)
	if enabled != d.Enabled || len(deps) != len(d.DependsOn) {
		return errors.New("monitoring_configuration_definition_mismatch")
	}
	for n, key := range deps {
		if keys[key] == "" || keys[key] != d.DependsOn[n] {
			return errors.New("monitoring_configuration_definition_mismatch")
		}
	}
	if ref, ok := p["configuration_ref"]; ok {
		parts := configurationReference.FindStringSubmatch(configurationString(ref))
		if parts == nil || parts[1] != strconv.FormatInt(a.Definition.ClientID, 10) || parts[2] != strconv.FormatInt(i.SourceTemplateID, 10) || parts[3] != strconv.FormatInt(i.SourceItemID, 10) || parts[4] != configurationString(p["tipo"]) {
			return errors.New("monitoring_configuration_binding_mismatch")
		}
	}
	return nil
}

func configurationString(v ConfigurationValue) string {
	var s string
	_ = json.Unmarshal(v.Value, &s)
	return s
}
func configurationNumber(v ConfigurationValue) int64 {
	var n int64
	_ = json.Unmarshal(v.Value, &n)
	return n
}
func configurationKey(a DefinitionAuthority, id int64, source, metric string) string {
	return fmt.Sprintf("v1:c:%d:a:%d:check:%d:%s:%s", a.ClientID, a.AssetID, id, source, metric)
}
