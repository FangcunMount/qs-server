package options

import (
	"fmt"
	"sort"
)

// FieldSchema describes known nested fields. A nil child marks a leaf.
type FieldSchema map[string]FieldSchema

// ValidateRawSection rejects unknown fields in one optional top-level section.
func ValidateRawSection(settings map[string]any, section string, schema FieldSchema) error {
	raw, ok := settings[section]
	if !ok {
		return nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", section)
	}
	return validateFields(section, values, schema)
}

func validateFields(path string, values map[string]any, schema FieldSchema) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		child, known := schema[key]
		if !known {
			return fmt.Errorf("unknown configuration field %s.%s", path, key)
		}
		if child == nil {
			continue
		}
		nested, ok := values[key].(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%s must be an object", path, key)
		}
		if err := validateFields(path+"."+key, nested, child); err != nil {
			return err
		}
	}
	return nil
}
