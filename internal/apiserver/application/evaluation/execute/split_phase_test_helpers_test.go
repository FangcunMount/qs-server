package execute

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ExecutionInput = evalpipeline.ExecutionInput

type countingEvaluator struct {
	key     evaluation.ExecutionIdentity
	calls   int
	outcome *domainoutcome.Execution
}

type evaluatorStub struct {
	key     evaluation.ExecutionIdentity
	execute func(context.Context, ExecutionInput) (*domainoutcome.Execution, error)
}

func (e evaluatorStub) ExecutionIdentity() evaluation.ExecutionIdentity { return e.key }
func (e evaluatorStub) Key() evaluation.ExecutionIdentity               { return e.key }
func (e evaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*domainoutcome.Execution, error) {
	if e.execute != nil {
		return e.execute(ctx, input)
	}
	return domainoutcome.NewExecution(domainoutcome.ModelRef{}, domainoutcome.Summary{}, domainoutcome.Detail{}), nil
}

type testEvaluator interface {
	ExecutionIdentity() evaluation.ExecutionIdentity
	Execute(context.Context, ExecutionInput) (*domainoutcome.Execution, error)
}

type evaluatorDescriptorExecutor struct{ evaluator testEvaluator }

func (e evaluatorDescriptorExecutor) Execute(ctx context.Context, _ evalpipeline.RuntimeDescriptor, input ExecutionInput) (*domainoutcome.Execution, error) {
	return e.evaluator.Execute(ctx, input)
}

func withTestEvaluator(evaluator testEvaluator) EngineOption {
	return func(s *service) {
		identity := evaluator.ExecutionIdentity()
		family, ok := modelcatalog.AlgorithmFamilyFromIdentity(identity.Kind, identity.SubKind, identity.Algorithm)
		if !ok {
			panic("test evaluator has unsupported execution identity: " + identity.String())
		}
		registry := evalpipeline.NewRuntimeDescriptorRegistry()
		if err := registry.Register(evalpipeline.RuntimeDescriptor{
			Key: evalpipeline.DescriptorKey{AlgorithmFamily: family}, AlgorithmFamily: family,
		}); err != nil {
			panic(err)
		}
		s.descriptorRegistry = registry
		s.descriptorExecutor = evaluatorDescriptorExecutor{evaluator: evaluator}
	}
}

func (e *countingEvaluator) ExecutionIdentity() evaluation.ExecutionIdentity { return e.key }
func (e *countingEvaluator) Key() evaluation.ExecutionIdentity               { return e.key }
func (e *countingEvaluator) Execute(context.Context, ExecutionInput) (*domainoutcome.Execution, error) {
	e.calls++
	if e.outcome != nil {
		return e.outcome, nil
	}
	return domainoutcome.NewExecution(
		domainoutcome.ModelRef{ModelKind: "scale", ModelCode: "SCALE-1", ModelVersion: "1.0.0", ModelTitle: "scale"},
		domainoutcome.Summary{PrimaryLabel: "recomputed"}, domainoutcome.Detail{Kind: "scale"},
	), nil
}

type stubInputResolver struct{}

func (stubInputResolver) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return &evaluationinput.InputSnapshot{}, nil
}

type splitPhaseCapture struct {
	CommitCalls int
	Request     outcomecommit.CommitRequest
}

type recordingEvaluationCommitter struct {
	capture *splitPhaseCapture
}

func (c *recordingEvaluationCommitter) Commit(_ context.Context, request outcomecommit.CommitRequest) (*domainoutcome.Record, error) {
	c.capture.CommitCalls++
	c.capture.Request = request
	if request.Assessment != nil && request.Execution != nil {
		if err := request.Assessment.ApplyScoringProjectionAt(evaloutcome.ScoringProjectionFromExecution(request.Execution), request.EvaluatedAt); err != nil {
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

func executionForAssessment(a *assessment.Assessment, label string) *domainoutcome.Execution {
	modelRef := domainoutcome.ModelRef{}
	if a != nil && a.EvaluationModelRef() != nil {
		modelRef = evaloutcome.ModelRefFromAssessment(*a.EvaluationModelRef())
	}
	return domainoutcome.NewExecution(modelRef, domainoutcome.Summary{PrimaryLabel: label}, domainoutcome.Detail{Kind: modelRef.Kind()})
}

var _ outcomecommit.Committer = (*recordingEvaluationCommitter)(nil)
