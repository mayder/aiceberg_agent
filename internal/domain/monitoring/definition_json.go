package monitoring

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// DecodeDefinition bounds wire input and rejects missing/unknown/duplicate fields.
// Authority and semantics must still be validated with ValidateDefinition.
func DecodeDefinition(raw []byte) (Definition, error) {
	fail := errors.New("invalid_monitoring_definition")
	if len(raw) > MaxDefinitionBytes || len(raw) == 0 {
		return Definition{}, fail
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := definitionJSONValue(decoder, 0); err != nil {
		return Definition{}, fail
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Definition{}, fail
	}
	if err := definitionJSONShape(raw); err != nil {
		return Definition{}, err
	}
	var definition Definition
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&definition) != nil {
		return Definition{}, fail
	}
	return definition, nil
}

func definitionJSONShape(raw []byte) error {
	fail := errors.New("invalid_monitoring_definition")
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || !definitionFields(object, "schema_version", "revision", "client_id", "asset_id", "items") {
		return fail
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(object["items"], &items) != nil || len(items) < 1 || len(items) > MaxDefinitionItems {
		return fail
	}
	for _, item := range items {
		if !definitionFields(item, "key", "check_id", "metric", "source", "kind", "value_type", "unit", "freshness_sec", "history_days", "trends_days", "enabled", "depends_on") {
			return errors.New("invalid_monitoring_item")
		}
		for _, v := range item {
			if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
				return errors.New("invalid_monitoring_item")
			}
		}
	}
	return nil
}

func definitionFields(object map[string]json.RawMessage, keys ...string) bool {
	if len(object) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := object[key]; !ok {
			return false
		}
	}
	return true
}

func definitionJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("invalid_monitoring_definition")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return errors.New("invalid_monitoring_definition")
			}
			keys[name] = true
		}
		if err := definitionJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if (delimiter == '{' && end != json.Delim('}')) || (delimiter == '[' && end != json.Delim(']')) {
		return errors.New("invalid_monitoring_definition")
	}
	return nil
}
