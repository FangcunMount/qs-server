package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ToExecution maps factor-scoring output to the canonical Evaluation execution result.
func ToExecution(
	result *calcscoring.Result,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *domainoutcome.Execution {
	return calculationadapter.ExecutionFromScoringInterpretation(result, evaloutcome.ModelRefFromAssessment(scaleModelRef(a, snapshot)))
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
