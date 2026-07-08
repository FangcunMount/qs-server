package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainfactor_scoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ToAssessmentOutcome 映射scale interpretation 结果 为 规范 领域结果。
func ToAssessmentOutcome(
	result *domainfactor_scoring.ScaleInterpretationResult,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *assessment.AssessmentOutcome {
	return calculationadapter.AssessmentOutcomeFromScaleInterpretation(result, scaleModelRef(a, snapshot))
}

func scaleModelRef(a *assessment.Assessment, snapshot *evaluationinput.InputSnapshot) assessment.EvaluationModelRef {
	if a != nil && a.EvaluationModelRef() != nil {
		return *a.EvaluationModelRef()
	}
	if snapshot != nil && snapshot.Model != nil {
		return assessment.NewEvaluationModelRefByCode(
			assessment.EvaluationModelKind(snapshot.Model.Kind),
			meta.NewCode(snapshot.Model.Code),
			snapshot.Model.Version,
			snapshot.Model.Title,
		)
	}
	return assessment.EvaluationModelRef{}
}
