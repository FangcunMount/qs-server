package execute

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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
	CommitCalls int
	Outcome     evaloutcome.Outcome
}

type recordingEvaluationCommitter struct {
	capture *splitPhaseCapture
}

func (c *recordingEvaluationCommitter) Commit(_ context.Context, request outcomecommit.Request) (*domainoutcome.Record, error) {
	c.capture.CommitCalls++
	c.capture.Outcome = request.Outcome
	if request.Outcome.Assessment != nil && request.Outcome.Execution != nil {
		if err := request.Outcome.Assessment.ApplyScoringOutcome(evaloutcome.AssessmentOutcomeFromExecution(request.Outcome.Execution)); err != nil {
			return nil, err
		}
	}
	if request.Run != nil {
		if err := request.Run.Succeed(request.EvaluatedAt); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func newSplitPhaseTestService(
	repo assessment.Repository,
	input evaluationinput.Resolver,
	capture *splitPhaseCapture,
	opts ...EngineOption,
) Engine {
	base := []EngineOption{
		WithEvaluationCommitter(&recordingEvaluationCommitter{capture: capture}),
		WithRunRepository(&stubRunRepo{}),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, &engineRecordingEventStager{}),
	}
	return NewEngine(repo, input, append(base, opts...)...)
}

func splitPhaseAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(9001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(8001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "", "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return a
}

var _ outcomecommit.Committer = (*recordingEvaluationCommitter)(nil)
