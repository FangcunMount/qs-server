// Package template owns report template selection and rendering contracts.
package template

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	typologytemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
)

// Profile is a mechanism-neutral report template profile.
type Profile struct {
	Kind             string
	DefaultModelName string
	DefaultModelCode string
}

// Renderer renders a report template into domain report structures.
type Renderer interface {
	Profile() Profile
}

// PersonalityTypeTemplate is the typology personality-type presentation template.
type PersonalityTypeTemplate = typologytemplate.PersonalityTypeReportTemplate

// TraitProfileTemplate is the typology trait-profile presentation template.
type TraitProfileTemplate = typologytemplate.TraitProfileReportTemplate

// BuildPersonalityTypeReport renders a personality-type report from mechanism-neutral detail.
var BuildPersonalityTypeReport = typologytemplate.BuildPersonalityTypeReport

// BuildTraitProfileReport renders a trait-profile report from mechanism-neutral detail.
var BuildTraitProfileReport = typologytemplate.BuildTraitProfileReport

// DefaultReportBuilder is the shared default report composer.
type DefaultReportBuilder = domainreport.DefaultReportBuilder

var NewDefaultInterpretReportBuilder = domainreport.NewDefaultInterpretReportBuilder
