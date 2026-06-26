package result

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Outcome struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
	Execution  *assessment.AssessmentOutcome
}

// LegacyResult projects the canonical outcome into the legacy write model.
func (o Outcome) LegacyResult() *assessment.EvaluationResult {
	if o.Execution == nil {
		return nil
	}
	return o.Execution.ToEvaluationResult()
}

// NewOutcomeFromLegacyResult adapts a legacy evaluation result for tests and compatibility callers.
func NewOutcomeFromLegacyResult(a *assessment.Assessment, input *evaluationinput.InputSnapshot, result *assessment.EvaluationResult) Outcome {
	return Outcome{
		Assessment: a,
		Input:      input,
		Execution:  assessment.AssessmentOutcomeFromEvaluationResult(result),
	}
}

type Writer interface {
	Write(ctx context.Context, outcome Outcome) error
}

type ScoreProjector interface {
	Key() evaluation.EvaluatorKey
	Project(ctx context.Context, outcome Outcome) error
}

type ReportBuilder interface {
	Key() evaluation.EvaluatorKey
	ReportType() domainReport.ReportType
	Build(ctx context.Context, outcome Outcome) (*domainReport.InterpretReport, error)
}
