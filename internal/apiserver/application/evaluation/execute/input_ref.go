package execute

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// inputRefFromAssessment builds the immutable input lookup reference for an assessment.
func inputRefFromAssessment(a *assessment.Assessment, assessmentID uint64) evaluationinput.InputRef {
	modelRef := modelRefFromAssessment(a)
	ref := evaluationinput.InputRef{
		AssessmentID:         assessmentID,
		ModelRef:             modelRef,
		AnswerSheetID:        a.AnswerSheetRef().ID().Uint64(),
		QuestionnaireCode:    a.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: a.QuestionnaireRef().Version(),
		TesteeID:             a.TesteeID().Uint64(),
	}
	if submittedAt := a.SubmittedAt(); submittedAt != nil {
		ref.AsOf = submittedAt.UTC()
	}
	return ref
}

func modelRefFromAssessment(a *assessment.Assessment) evaluationinput.ModelRef {
	if a == nil || a.EvaluationModelRef() == nil {
		return evaluationinput.ModelRef{}
	}
	ref := a.EvaluationModelRef()
	return evaluationinput.ModelRef{
		Kind:      evaluationinput.EvaluationModelKind(ref.Kind().String()),
		Algorithm: string(ref.Algorithm()),
		Code:      ref.Code().String(),
		Version:   ref.Version(),
		Title:     ref.Title(),
	}
}

func mapScaleNotFoundError(err error) error {
	return evalerrors.MedicalScaleNotFound(err, "量表不存在")
}
