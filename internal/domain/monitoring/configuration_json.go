package monitoring

import (
	"encoding/json"
	"errors"
)

func DecodeConfiguration(raw []byte, a ConfigurationAuthority) (Configuration, error) {
	fail := errors.New("invalid_monitoring_configuration")
	if len(raw) == 0 || len(raw) > MaxDefinitionBytes || !recordJSONValid(raw) {
		return Configuration{}, fail
	}
	object, ok := configurationObject(raw, "contract", "schema_version", "definition", "root", "lineage", "items")
	if !ok {
		return Configuration{}, fail
	}
	if _, ok = configurationObject(object["root"], "id", "revision"); !ok {
		return Configuration{}, fail
	}
	var refs []json.RawMessage
	if json.Unmarshal(object["lineage"], &refs) != nil || refs == nil || len(refs) > 64 {
		return Configuration{}, fail
	}
	for _, ref := range refs {
		if _, ok = configurationObject(ref, "id", "revision"); !ok {
			return Configuration{}, fail
		}
	}
	if _, err := DecodeDefinition(object["definition"]); err != nil {
		return Configuration{}, err
	}
	if !configurationItemsShape(object["items"]) {
		return Configuration{}, fail
	}
	var c Configuration
	if json.Unmarshal(raw, &c) != nil {
		return Configuration{}, fail
	}
	if err := ValidateConfiguration(c, a); err != nil {
		return Configuration{}, err
	}
	return c, nil
}

func configurationObject(raw []byte, keys ...string) (map[string]json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || !definitionFields(object, keys...) {
		return nil, false
	}
	for _, value := range object {
		if string(value) == "null" {
			return nil, false
		}
	}
	return object, true
}

func configurationItemsShape(raw []byte) bool {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil || items == nil || len(items) < 1 || len(items) > 128 {
		return false
	}
	for _, item := range items {
		object, ok := configurationObject(item, "template_key", "item_key", "source_item_id", "source_template_id", "source_revision", "parameters")
		if !ok {
			return false
		}
		var params map[string]json.RawMessage
		if json.Unmarshal(object["parameters"], &params) != nil || params == nil || len(params) > 24 {
			return false
		}
		for _, value := range params {
			if _, ok = configurationObject(value, "type", "value"); !ok {
				return false
			}
		}
	}
	return true
}

func EncodeConfiguration(c Configuration, a ConfigurationAuthority) ([]byte, error) {
	if err := ValidateConfiguration(c, a); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > MaxDefinitionBytes {
		return nil, errors.New("invalid_monitoring_configuration")
	}
	return raw, nil
}

// Restores create a new revision; both histories require independent authority.
func ValidateConfigurationEvolution(old, next Configuration, before, after ConfigurationAuthority) error {
	if err := ValidateConfiguration(old, before); err != nil {
		return err
	}
	if err := ValidateConfiguration(next, after); err != nil {
		return err
	}
	if before.Definition.ClientID != after.Definition.ClientID || before.Definition.AssetID != after.Definition.AssetID {
		return errors.New("monitoring_configuration_scope_mismatch")
	}
	if old.Root.ID != next.Root.ID {
		return errors.New("monitoring_configuration_scope_mismatch")
	}
	return validateDefinitionSeriesEvolution(old.Definition, next.Definition)
}
