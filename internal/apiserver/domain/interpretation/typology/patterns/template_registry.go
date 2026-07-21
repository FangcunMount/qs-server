package patterns

import (
	"errors"
	"fmt"
)

// ReportSpec is the report-template selection input owned by the interpretation
// boundary. Runtime payload packages map into it at adapter edges.
type ReportSpec struct {
	AdapterKey ReportAdapterKey
	TemplateID string
}

// ReportAdapterKey selects a built-in report template adapter.
type ReportAdapterKey string

const (
	ReportAdapterPersonalityType ReportAdapterKey = "personality_type"
	ReportAdapterTraitProfile    ReportAdapterKey = "trait_profile"
)

// ErrUnknownTemplateID marks a non-empty TemplateID that is not registered.
// Empty TemplateID may still fall back to AdapterKey defaults.
var ErrUnknownTemplateID = errors.New("unknown_template_id")

// PersonalityTypeTemplateByID resolves a personality-type report template factory by TemplateID.
func PersonalityTypeTemplateByID(templateID string) (PersonalityTypeReportTemplate, bool) {
	switch templateID {
	case "mbti":
		return MBTIPersonalityTypeTemplate(), true
	case "sbti":
		return SBTIPersonalityTypeTemplate(), true
	default:
		return PersonalityTypeReportTemplate{}, false
	}
}

// TraitProfileTemplateByID resolves a trait-profile report template factory by TemplateID.
func TraitProfileTemplateByID(templateID string) (TraitProfileReportTemplate, bool) {
	switch templateID {
	case "bigfive":
		return BigFiveTraitProfileTemplate(), true
	default:
		return TraitProfileReportTemplate{}, false
	}
}

// IsRegisteredTemplateID reports whether templateID is known to either registry.
func IsRegisteredTemplateID(templateID string) bool {
	if templateID == "" {
		return false
	}
	if _, ok := PersonalityTypeTemplateByID(templateID); ok {
		return true
	}
	_, ok := TraitProfileTemplateByID(templateID)
	return ok
}

// PersonalityTypeTemplateForSpec resolves templates from report spec.
// Empty TemplateID may fall back to AdapterKey defaults; a non-empty unknown
// TemplateID fails closed with ErrUnknownTemplateID.
func PersonalityTypeTemplateForSpec(spec ReportSpec) (PersonalityTypeReportTemplate, error) {
	if spec.TemplateID != "" {
		if tmpl, ok := PersonalityTypeTemplateByID(spec.TemplateID); ok {
			return tmpl, nil
		}
		return PersonalityTypeReportTemplate{}, fmt.Errorf("%w: %s", ErrUnknownTemplateID, spec.TemplateID)
	}
	return PersonalityTypeReportTemplate{}, nil
}

// TraitProfileTemplateForSpec resolves templates from report spec.
// Empty TemplateID may fall back to AdapterKey defaults; a non-empty unknown
// TemplateID fails closed with ErrUnknownTemplateID.
func TraitProfileTemplateForSpec(spec ReportSpec) (TraitProfileReportTemplate, error) {
	if spec.TemplateID != "" {
		if tmpl, ok := TraitProfileTemplateByID(spec.TemplateID); ok {
			return tmpl, nil
		}
		return TraitProfileReportTemplate{}, fmt.Errorf("%w: %s", ErrUnknownTemplateID, spec.TemplateID)
	}
	return TraitProfileReportTemplate{}, nil
}
