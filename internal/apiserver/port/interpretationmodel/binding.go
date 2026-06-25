package interpretationmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
)

// AssessmentBinding 建测评时从问卷解析出的解释模型绑定（含 scale legacy 字段）。
type AssessmentBinding struct {
	Ref              ModelRef
	MedicalScaleID   *uint64
	MedicalScaleCode *string
	MedicalScaleName *string
	ScaleVersion     *string
}

// AssessmentBindingResolver 统一解析问卷 -> 解释模型绑定。
type AssessmentBindingResolver interface {
	QuestionnaireModelBindingResolver
	ResolveAssessmentBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (AssessmentBinding, bool, error)
}

func ScaleAssessmentBinding(ref ModelRef, scaleID uint64, code, title, version string) AssessmentBinding {
	return AssessmentBinding{
		Ref:              ref,
		MedicalScaleID:   &scaleID,
		MedicalScaleCode: &code,
		MedicalScaleName: &title,
		ScaleVersion:     &version,
	}
}

func InterpretationAssessmentBinding(ref ModelRef) AssessmentBinding {
	return AssessmentBinding{Ref: ref}
}

func (b AssessmentBinding) ModelKind() domain.ModelKind {
	return b.Ref.Kind
}
