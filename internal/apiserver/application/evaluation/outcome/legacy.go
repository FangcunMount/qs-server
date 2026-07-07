package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// LegacyResult projects the canonical outcome into the legacy write model.
func LegacyResult(o Outcome) *assessment.EvaluationResult {
	if o.Execution == nil {
		return nil
	}
	return o.Execution.ToEvaluationResult()
}

// NewOutcomeFromLegacyResult adapts a legacy evaluation result for tests and compatibility callers.
func NewOutcomeFromLegacyResult(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
	result *assessment.EvaluationResult,
) Outcome {
	outcome := Outcome{
		Assessment: a,
		Input:      input,
		Execution:  assessment.AssessmentOutcomeFromEvaluationResult(result), //nolint:staticcheck // single boundary adapter for characterization
	}
	if snapshot, ok := PublishedSnapshotFromInput(input); ok {
		if key, err := evalpipeline.RuntimeDescriptorKeyFromSnapshot(snapshot); err == nil {
			outcome.RuntimeDescriptorKey = key
		}
	}
	return outcome
}
