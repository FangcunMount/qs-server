package execute

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type countingEvaluator struct {
	key     evaluation.ExecutionIdentity
	calls   int
	outcome *assessment.AssessmentOutcome
}

func (e *countingEvaluator) ExecutionIdentity() evaluation.ExecutionIdentity { return e.key }
func (e *countingEvaluator) Key() evaluation.ExecutionIdentity               { return e.key }
func (e *countingEvaluator) Execute(context.Context, ExecutionInput) (*domainoutcome.Execution, error) {
	e.calls++
	if e.outcome != nil {
		return evaloutcome.ExecutionFromAssessmentOutcome(e.outcome), nil
	}
	return evaloutcome.ExecutionFromAssessmentOutcome(assessment.NewAssessmentOutcome(
		assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindScale, meta.NewCode("SCALE-1"), "1.0.0", "scale"),
		assessment.ResultSummary{PrimaryLabel: "recomputed"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)), nil
}

type stubInputResolver struct{}

func (stubInputResolver) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return &evaluationinput.InputSnapshot{}, nil
}

type splitPhaseCapture struct {
	ScoringCalls int
	Outcome      evaloutcome.Outcome
}

type recordingSplitPhaseScoringWriter struct {
	capture *splitPhaseCapture
}

func (w *recordingSplitPhaseScoringWriter) Write(_ context.Context, outcome evaloutcome.Outcome) error {
	w.capture.ScoringCalls++
	w.capture.Outcome = outcome
	if outcome.Assessment != nil && outcome.Execution != nil {
		return outcome.Assessment.ApplyScoringOutcome(evaloutcome.AssessmentOutcomeFromExecution(outcome.Execution))
	}
	return nil
}

func newSplitPhaseTestService(
	repo assessment.Repository,
	input evaluationinput.Resolver,
	capture *splitPhaseCapture,
	opts ...ServiceOption,
) Service {
	base := []ServiceOption{
		WithScoringWriter(&recordingSplitPhaseScoringWriter{capture: capture}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	}
	return NewService(repo, input, append(base, opts...)...)
}

var _ outcomescoring.Writer = (*recordingSplitPhaseScoringWriter)(nil)
