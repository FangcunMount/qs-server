package modelcatalog

import "fmt"

const legacyScaleBindingSourceKey = "legacy_scale"

// LegacyScaleBinding contains the remaining non-semantic fields needed to
// project published assessment models to old scale-facing contracts. Runtime
// model resolution is always anchored by Code and Version.
type LegacyScaleBinding struct {
	MedicalScaleID uint64 `bson:"medical_scale_id,omitempty" json:"medical_scale_id,omitempty"`
	ScaleVersion   string `bson:"scale_version,omitempty" json:"scale_version,omitempty"`
}

func LegacyScaleBindingFromPublished(model *PublishedModel) (LegacyScaleBinding, bool) {
	if model == nil || model.Source == nil {
		return LegacyScaleBinding{}, false
	}
	raw, ok := model.Source[legacyScaleBindingSourceKey]
	if !ok || raw == nil {
		return LegacyScaleBinding{}, false
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return LegacyScaleBinding{}, false
	}
	binding := LegacyScaleBinding{}
	if value, ok := uint64Value(values["medical_scale_id"]); ok {
		binding.MedicalScaleID = value
	}
	if value, ok := values["scale_version"].(string); ok {
		binding.ScaleVersion = value
	}
	return binding, binding.MedicalScaleID != 0 || binding.ScaleVersion != ""
}

func SetLegacyScaleBinding(model *PublishedModel, binding LegacyScaleBinding) {
	if model == nil {
		return
	}
	if model.Source == nil {
		model.Source = map[string]any{}
	}
	model.Source[legacyScaleBindingSourceKey] = map[string]any{
		"medical_scale_id": binding.MedicalScaleID,
		"scale_version":    binding.ScaleVersion,
	}
}

func uint64Value(value any) (uint64, bool) {
	switch item := value.(type) {
	case uint64:
		return item, true
	case uint32:
		return uint64(item), true
	case int64:
		if item >= 0 {
			return uint64(item), true
		}
	case int32:
		if item >= 0 {
			return uint64(item), true
		}
	case int:
		if item >= 0 {
			return uint64(item), true
		}
	case float64:
		if item >= 0 && item == float64(uint64(item)) {
			return uint64(item), true
		}
	case string:
		var parsed uint64
		if _, err := fmt.Sscan(item, &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}
