package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// LegacyResult 投影规范 结果 为 旧写模型。
func LegacyResult(o Outcome) *assessment.EvaluationResult {
	if o.Execution == nil {
		return nil
	}
	execution := o.Execution
	result := assessment.NewModelEvaluationResult(
		AssessmentModelRefFromExecution(execution.ModelRef),
		assessment.ResultSummary{
			PrimaryLabel: execution.Summary.PrimaryLabel,
			Score:        cloneFloat(execution.Summary.Score),
			Level:        cloneString(execution.Summary.Level),
			Tags:         append([]string(nil), execution.Summary.Tags...),
		},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKind(execution.Detail.Kind), Payload: execution.Detail.Payload},
	)
	if execution.Primary != nil {
		result.TotalScore = execution.Primary.Value
	}
	if execution.Level != nil && assessment.IsRiskLevelCode(execution.Level.Code) {
		result.RiskLevel = assessment.RiskLevel(execution.Level.Code)
	}
	if scores, ok := execution.Detail.Payload.([]assessment.FactorScoreResult); ok {
		result.FactorScores = append([]assessment.FactorScoreResult(nil), scores...)
	}
	return result
}

// NewOutcomeFromLegacyResult 适配旧版 评估 结果 用于 tests 和 兼容性 callers。
func NewOutcomeFromLegacyResult(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
	result *assessment.EvaluationResult,
) Outcome {
	execution := domainoutcome.NewExecution(
		ModelRefFromAssessment(result.ModelRef),
		domainoutcome.Summary{
			PrimaryLabel: result.Summary.PrimaryLabel,
			Score:        cloneFloat(result.Summary.Score),
			Level:        cloneString(result.Summary.Level),
			Tags:         append([]string(nil), result.Summary.Tags...),
		},
		domainoutcome.Detail{Kind: result.Detail.Kind, Payload: result.Detail.Payload},
	)
	if execution.Detail.Payload == nil && len(result.FactorScores) > 0 {
		execution.Detail.Payload = append([]assessment.FactorScoreResult(nil), result.FactorScores...)
	}
	if result.Summary.Score != nil || result.TotalScore != 0 {
		score := result.TotalScore
		if result.Summary.Score != nil {
			score = *result.Summary.Score
		}
		execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: score}
	}
	if result.Summary.Level != nil && *result.Summary.Level != "" {
		execution.Level = &domainoutcome.ResultLevel{Code: *result.Summary.Level, Label: result.Summary.PrimaryLabel}
	} else if result.RiskLevel != "" {
		execution.Level = &domainoutcome.ResultLevel{Code: string(result.RiskLevel), Label: result.Summary.PrimaryLabel}
	}
	outcome := Outcome{
		Assessment: a,
		Input:      input,
		Execution:  execution,
	}
	if route, ok := ModelRouteFromInput(input); ok {
		if key, err := evalpipeline.RuntimeDescriptorKeyFromRoute(route); err == nil {
			outcome.RuntimeDescriptorKey = key
		}
	}
	return outcome
}
