package scale

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ResultMapper interface {
	ToEvaluationResult(
		result *scaleinterpretation.ScaleInterpretationResult,
		a *assessment.Assessment,
		snapshot *evaluationinput.InputSnapshot,
	) *assessment.EvaluationResult
}

type DefaultResultMapper struct{}

func (DefaultResultMapper) ToEvaluationResult(
	result *scaleinterpretation.ScaleInterpretationResult,
	a *assessment.Assessment,
	snapshot *evaluationinput.InputSnapshot,
) *assessment.EvaluationResult {
	if result == nil {
		return nil
	}
	factorScores := make([]assessment.FactorScoreResult, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, assessment.NewFactorScoreResult(
			assessment.NewFactorCode(string(fs.FactorCode)),
			fs.FactorName,
			fs.RawScore,
			assessment.RiskLevel(fs.RiskLevel),
			fs.Conclusion,
			fs.Suggestion,
			fs.IsTotalScore,
		))
	}
	evalResult := assessment.NewEvaluationResult(
		result.TotalScore,
		assessment.RiskLevel(result.RiskLevel),
		result.Conclusion,
		result.Suggestion,
		factorScores,
	)
	if a != nil && a.EvaluationModelRef() != nil {
		evalResult.WithModelRef(*a.EvaluationModelRef())
	} else if snapshot != nil && snapshot.Model != nil {
		modelRef := assessment.NewEvaluationModelRefByCode(
			assessment.EvaluationModelKind(snapshot.Model.Kind),
			meta.NewCode(snapshot.Model.Code),
			snapshot.Model.Version, snapshot.Model.Title,
		)
		evalResult.WithModelRef(modelRef)
	}
	return evalResult
}
