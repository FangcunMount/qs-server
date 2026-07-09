package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// LegacyResult 投影规范 结果 为 旧写模型。
func LegacyResult(o Outcome) *assessment.EvaluationResult {
	if o.Execution == nil {
		return nil
	}
	return o.Execution.ToEvaluationResult()
}

// NewOutcomeFromLegacyResult 适配旧版 评估 结果 用于 tests 和 兼容性 callers。
func NewOutcomeFromLegacyResult(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
	result *assessment.EvaluationResult,
) Outcome {
	outcome := Outcome{
		Assessment: a,
		Input:      input,
		Execution:  assessment.AssessmentOutcomeFromEvaluationResult(result), //nolint:staticcheck // 单一表征边界适配器
	}
	if route, ok := ModelRouteFromInput(input); ok {
		if key, err := evalpipeline.RuntimeDescriptorKeyFromRoute(route); err == nil {
			outcome.RuntimeDescriptorKey = key
		}
	}
	return outcome
}
