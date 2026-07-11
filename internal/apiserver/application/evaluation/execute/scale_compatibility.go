package execute

import (
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// inputRefFromAssessment 从评估数据中获取评估输入引用
func inputRefFromAssessment(a *assessment.Assessment, assessmentID uint64) evaluationinput.InputRef {
	modelRef := modelRefFromAssessment(a)
	return evaluationinput.InputRef{
		AssessmentID:         assessmentID,
		ModelRef:             modelRef,
		AnswerSheetID:        a.AnswerSheetRef().ID().Uint64(),
		QuestionnaireCode:    a.QuestionnaireRef().Code().String(),
		QuestionnaireVersion: a.QuestionnaireRef().Version(),
	}
}

// modelRefFromAssessment 从评估数据中获取评估模型引用
func modelRefFromAssessment(a *assessment.Assessment) evaluationinput.ModelRef {
	if a == nil || a.EvaluationModelRef() == nil {
		return evaluationinput.ModelRef{}
	}
	ref := a.EvaluationModelRef()
	return evaluationinput.ModelRef{
		Kind:      evaluationinput.EvaluationModelKind(ref.Kind().String()),
		SubKind:   string(ref.SubKind()),
		Algorithm: string(ref.Algorithm()),
		Code:      ref.Code().String(),
		Version:   ref.Version(),
		Title:     ref.Title(),
	}
}

// mapScaleInputResolveError 映射量表输入解析错误
func mapScaleInputResolveError(err error) error {
	return evalerrors.MedicalScaleNotFound(err, "量表不存在")
}
