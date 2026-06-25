package assessmentmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

// AssessmentBinding 建测评时从问卷解析出的测评模型绑定（含 scale legacy 字段）。
type AssessmentBinding struct {
	Ref              Ref
	MedicalScaleID   *uint64
	MedicalScaleCode *string
	MedicalScaleName *string
	ScaleVersion     *string
}

// AssessmentBindingResolver 统一解析问卷 -> 测评模型绑定。
type AssessmentBindingResolver interface {
	QuestionnaireResolver
	ResolveAssessmentBinding(ctx context.Context, questionnaireCode, questionnaireVersion string) (AssessmentBinding, bool, error)
}

func ScaleAssessmentBinding(ref Ref, scaleID uint64, code, title, version string) AssessmentBinding {
	return AssessmentBinding{
		Ref:              ref,
		MedicalScaleID:   &scaleID,
		MedicalScaleCode: &code,
		MedicalScaleName: &title,
		ScaleVersion:     &version,
	}
}

func RuleSetAssessmentBinding(ref Ref) AssessmentBinding {
	return AssessmentBinding{Ref: ref}
}

func (b AssessmentBinding) ModelKind() domain.Kind {
	return b.Ref.Kind
}
