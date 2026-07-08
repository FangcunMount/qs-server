// Package template 负责report template 选择 和 rendering contracts。
package template

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/builder"
	typologytemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
)

// Profile 是机制无关 report template 画像。
type Profile struct {
	Kind             string
	DefaultModelName string
	DefaultModelCode string
}

// Renderer renders report template 为 领域报告结构。
type Renderer interface {
	Profile() Profile
}

// PersonalityTypeTemplate 是类型学 personality-type 呈现 template。
type PersonalityTypeTemplate = typologytemplate.PersonalityTypeReportTemplate

// TraitProfileTemplate 是类型学 trait-画像 呈现 template。
type TraitProfileTemplate = typologytemplate.TraitProfileReportTemplate

// BuildPersonalityTypeReport renders personality-type report 从 机制无关 detail。
var BuildPersonalityTypeReport = typologytemplate.BuildPersonalityTypeReport

// BuildTraitProfileReport renders trait-画像 report 从 机制无关 detail。
var BuildTraitProfileReport = typologytemplate.BuildTraitProfileReport

// DefaultReportBuilder 是共享默认 report composer。
type DefaultReportBuilder = builder.DefaultReportBuilder

var NewDefaultInterpretReportBuilder = builder.NewDefaultInterpretReportBuilder
