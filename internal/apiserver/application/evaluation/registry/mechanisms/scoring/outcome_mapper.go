package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
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
	return calculationadapter.ExecutionFromScoringInterpretation(result, scaleModelRef(a, snapshot))
}

// ToAssessmentOutcome remains as a source-compatible name during migration.
//
// Deprecated: use ToExecution.
func ToAssessmentOutcome(
	result *calcscoring.Result,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *domainoutcome.Execution {
	return ToExecution(result, a, snapshot)
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
