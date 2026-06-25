package ruleset

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset"
)

// AssessmentBinding 建测评时从问卷解析出的解释模型绑定（含 scale legacy 字段）。
type AssessmentBinding struct {
	Ref              RuleSetRef
	MedicalScaleID   *uint64
	MedicalScaleCode *string
	MedicalScaleName *string
	ScaleVersion     *string
}

// AssessmentBindingResolver 统一解析问卷 -> 解释模型绑定。
type AssessmentBindingResolver interface {
	QuestionnaireRuleSetResolver
	ResolveAssessmentBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (AssessmentBinding, bool, error)
}

func ScaleAssessmentBinding(ref RuleSetRef, scaleID uint64, code, title, version string) AssessmentBinding {
	return AssessmentBinding{
		Ref:              ref,
		MedicalScaleID:   &scaleID,
		MedicalScaleCode: &code,
		MedicalScaleName: &title,
		ScaleVersion:     &version,
	}
}

func RuleSetAssessmentBinding(ref RuleSetRef) AssessmentBinding {
	return AssessmentBinding{Ref: ref}
}

func (b AssessmentBinding) ModelKind() domain.RuleSetKind {
	return b.Ref.Kind
}
