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

var (
	// ErrTemplateIDRequired marks a missing persisted template route.
	ErrTemplateIDRequired = errors.New("template_id_required")
	// ErrUnknownTemplateID marks a TemplateID that is not registered.
	ErrUnknownTemplateID = errors.New("unknown_template_id")
)

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
	case "enneagram":
		return EnneagramTraitProfileTemplate(), true
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

// PersonalityTypeTemplateForSpec resolves one explicitly selected template.
func PersonalityTypeTemplateForSpec(spec ReportSpec) (PersonalityTypeReportTemplate, error) {
	if spec.TemplateID == "" {
		return PersonalityTypeReportTemplate{}, ErrTemplateIDRequired
	}
	if tmpl, ok := PersonalityTypeTemplateByID(spec.TemplateID); ok {
		return tmpl, nil
	}
	return PersonalityTypeReportTemplate{}, fmt.Errorf("%w: %s", ErrUnknownTemplateID, spec.TemplateID)
}

// TraitProfileTemplateForSpec resolves one explicitly selected template.
func TraitProfileTemplateForSpec(spec ReportSpec) (TraitProfileReportTemplate, error) {
	if spec.TemplateID == "" {
		return TraitProfileReportTemplate{}, ErrTemplateIDRequired
	}
	if tmpl, ok := TraitProfileTemplateByID(spec.TemplateID); ok {
		return tmpl, nil
	}
	return TraitProfileReportTemplate{}, fmt.Errorf("%w: %s", ErrUnknownTemplateID, spec.TemplateID)
}
