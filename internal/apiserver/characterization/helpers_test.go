package characterization_test

import (
	"context"
	"fmt"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/testutil/interpretationinput"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	evaluationfactcodec "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact/codec"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// v1SplitPhaseConfig preserves the legacy characterization fixtures while keeping
// Evaluation and Interpretation as two explicit test-side use-case calls.
type v1SplitPhaseConfig struct {
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	ReportBuilder interpretationreporting.Builder

	Async          bool
	StageEvaluated func(ctx context.Context, events ...event.DomainEvent) error
}

func canonicalOutcome(t *testing.T, a *assessment.Assessment, input *evaluationinput.InputSnapshot, summary domainoutcome.Summary, detail domainoutcome.Detail) interpretationinput.PreviewOutcome {
	t.Helper()
	execution := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(*a.EvaluationModelRef()), summary, detail)
	if summary.Score != nil {
		execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: *summary.Score}
	}
	return previewOutcome(t, a, input, execution, evaluationfact.RuntimeIdentity{})
}

// buildV1SplitPhaseExecuteService mirrors container/modules/evaluation/assemble.go split-phase wiring.
// When repos are provided, the first repo is shared with the caller (cross-module harness).
func buildV1SplitPhaseExecuteService(t *testing.T, cfg v1SplitPhaseConfig, repos ...*charAssessmentRepo) (*charSplitPhaseService, *charSplitPhaseReportSaver) {
	t.Helper()

	var repo *charAssessmentRepo
	if len(repos) > 0 && repos[0] != nil {
		repo = repos[0]
	} else {
		repo = &charAssessmentRepo{assessment: cfg.Assessment}
	}
	reportSaver := &charSplitPhaseReportSaver{}
	committer := &charCapturingEvaluationCommitter{stage: cfg.StageEvaluated}

	reportBuilders, err := interpretationreporting.NewRegistry(cfg.ReportBuilder)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	runtimeDescriptorRegistry := wireV1RuntimeDescriptorRegistry(t)

	opts := []evaluationexecute.EngineOption{
		evaluationexecute.WithRuntimeDescriptorRegistry(runtimeDescriptorRegistry),
		evaluationexecute.WithEvaluationCommitter(committer),
		evaluationexecute.WithRunRepository(&charRunRepo{}),
		evaluationexecute.WithTransactionalOutbox(&charTxRunner{}, charEventStagerFunc(func(ctx context.Context, events ...event.DomainEvent) error {
			if cfg.StageEvaluated != nil {
				return cfg.StageEvaluated(ctx, events...)
			}
			return nil
		})),
	}

	core := evaluationexecute.NewEngine(
		repo,
		&charInputResolver{snapshot: cfg.Input},
		opts...,
	)
	return &charSplitPhaseService{
		Engine:       core,
		capture:      committer,
		inlineLegacy: !cfg.Async,
		generateReport: func(ctx context.Context, outcome interpretationinput.PreviewOutcome) error {
			input, err := interpretationinput.FromPreviewOutcome(outcome)
			if err != nil {
				return err
			}
			key, ok := interpretationreporting.KeyFromInput(input)
			if !ok {
				return fmt.Errorf("report builder mechanism key is required")
			}
			builder, err := reportBuilders.ResolveByMechanism(key)
			if err != nil {
				return err
			}
			draft, err := builder.Build(ctx, input)
			if err != nil {
				return err
			}
			return reportSaver.SaveDraft(draft)
		},
	}, reportSaver
}

type charCapturingEvaluationCommitter struct {
	stage   func(ctx context.Context, events ...event.DomainEvent) error
	outcome interpretationinput.PreviewOutcome
}

func (c *charCapturingEvaluationCommitter) Commit(ctx context.Context, request outcomecommit.CommitRequest) (*domainoutcome.Record, error) {
	if request.Assessment == nil || request.Execution == nil {
		return nil, nil
	}
	decoded, err := previewOutcomeFromExecution(request.Assessment, request.Input, request.Execution, evaluationfact.RuntimeIdentity{
		AlgorithmFamily: request.DescriptorKey.AlgorithmFamily, DecisionKind: request.DescriptorKey.DecisionKind, PayloadFormat: request.DescriptorKey.PayloadFormat,
	})
	if err != nil {
		return nil, err
	}
	c.outcome = decoded
	if err := request.Assessment.ApplyScoringProjectionAt(evaloutcome.ScoringProjectionFromExecution(request.Execution), request.EvaluatedAt); err != nil {
		return nil, err
	}
	if request.Run != nil {
		if err := request.Run.Succeed(request.EvaluatedAt); err != nil {
			return nil, err
		}
		request.Assessment.StageEvaluatedEvent(request.EvaluatedAt, meta.FromUint64(1), request.Run.ID())
	}
	if c.stage != nil {
		events := request.Assessment.Events()
		if err := c.stage(ctx, events...); err != nil {
			return nil, err
		}
	}
	request.Assessment.ClearEvents()
	return nil, nil
}

type charSplitPhaseService struct {
	evaluationexecute.Engine
	capture        *charCapturingEvaluationCommitter
	inlineLegacy   bool
	generateReport func(context.Context, interpretationinput.PreviewOutcome) error
}

func (s *charSplitPhaseService) Evaluate(ctx context.Context, assessmentID uint64) error {
	if err := s.Engine.Evaluate(ctx, assessmentID); err != nil {
		return err
	}
	if s.inlineLegacy {
		return s.GenerateReport(ctx, assessmentID)
	}
	return nil
}

func (s *charSplitPhaseService) GenerateReport(ctx context.Context, assessmentID uint64) error {
	if s.generateReport == nil {
		return nil
	}
	if err := s.generateReport(ctx, s.capture.outcome); err != nil {
		return err
	}
	return nil
}

type charAssessmentRepo struct {
	assessment *assessment.Assessment
}

func (r *charAssessmentRepo) Save(_ context.Context, a *assessment.Assessment) error {
	if a != nil {
		if a.ID().IsZero() {
			a.AssignID(assessment.NewID(7001))
		}
		r.assessment = a
	}
	return nil
}
func (r *charAssessmentRepo) FindByID(_ context.Context, id assessment.ID) (*assessment.Assessment, error) {
	if r.assessment != nil && r.assessment.ID() == id {
		return r.assessment, nil
	}
	return nil, nil
}
func (*charAssessmentRepo) Delete(context.Context, assessment.ID) error { return nil }
func (r *charAssessmentRepo) FindByAnswerSheetID(_ context.Context, ref assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	if r.assessment != nil && r.assessment.AnswerSheetRef() == ref {
		return r.assessment, nil
	}
	return nil, nil
}

type charRunRepo struct {
	latest *evalrun.EvaluationRun
}

func (r *charRunRepo) Claim(_ context.Context, request evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	run := evalrun.NewEvaluationRun(request.AssessmentID)
	if r.latest != nil {
		run = *r.latest
	}
	if run.Attempt().Status == evalrun.StatusRunning && run.HasActiveLease(request.ClaimedAt) {
		return evaluationrun.ClaimResult{Run: run}, nil
	}
	if err := run.Claim(evalrun.ClaimInput{Token: request.Token, TraceID: request.TraceID, ClaimedAt: request.ClaimedAt, LeaseExpiresAt: request.LeaseUntil}); err != nil {
		return evaluationrun.ClaimResult{}, err
	}
	copy := run
	r.latest = &copy
	return evaluationrun.ClaimResult{Run: run, Claimed: true}, nil
}
func (r *charRunRepo) SaveClaimed(_ context.Context, run evalrun.EvaluationRun) error {
	copy := run
	r.latest = &copy
	return nil
}
func (r *charRunRepo) FindLatestByAssessmentID(_ context.Context, _ uint64) (*evalrun.EvaluationRun, error) {
	return r.latest, nil
}
func (r *charRunRepo) ListByAssessmentID(_ context.Context, _ uint64, _ int) ([]evalrun.EvaluationRun, error) {
	if r.latest == nil {
		return nil, nil
	}
	return []evalrun.EvaluationRun{*r.latest}, nil
}
func (*charRunRepo) ListRetryableFailed(context.Context, evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	return &evaluationrun.ListRetryableFailedResult{}, nil
}

var _ evaluationrun.Repository = (*charRunRepo)(nil)

type charInputResolver struct {
	snapshot *evaluationinput.InputSnapshot
	lastRef  evaluationinput.InputRef
}

func (r *charInputResolver) Resolve(_ context.Context, ref evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	r.lastRef = ref
	return r.snapshot, nil
}

type charSplitPhaseCapture struct {
	interpretationCalls int
	outcome             interpretationinput.PreviewOutcome
}

type charRecordingInterpretation struct {
	cap *charSplitPhaseCapture
}

func (s *charRecordingInterpretation) GenerateAndPersist(_ context.Context, outcome interpretationinput.PreviewOutcome) error {
	s.cap.interpretationCalls++
	s.cap.outcome = outcome
	return nil
}

func newV1RecordingExecuteService(
	t *testing.T,
	a *assessment.Assessment,
	input *charInputResolver,
) (*charSplitPhaseService, *charSplitPhaseCapture) {
	t.Helper()
	capture := &charSplitPhaseCapture{}
	recording := &charCapturingEvaluationCommitter{}
	core := evaluationexecute.NewEngine(
		&charAssessmentRepo{assessment: a},
		input,
		evaluationexecute.WithRuntimeDescriptorRegistry(wireV1RuntimeDescriptorRegistry(t)),
		evaluationexecute.WithEvaluationCommitter(recording),
		evaluationexecute.WithRunRepository(&charRunRepo{}),
		evaluationexecute.WithTransactionalOutbox(&charTxRunner{}, charEventStagerFunc(func(context.Context, ...event.DomainEvent) error { return nil })),
	)
	return &charSplitPhaseService{
		Engine:       core,
		capture:      recording,
		inlineLegacy: true,
		generateReport: func(ctx context.Context, outcome interpretationinput.PreviewOutcome) error {
			return (&charRecordingInterpretation{cap: capture}).GenerateAndPersist(ctx, outcome)
		},
	}, capture
}

func previewOutcome(t *testing.T, a *assessment.Assessment, input *evaluationinput.InputSnapshot, execution *domainoutcome.Execution, runtime evaluationfact.RuntimeIdentity) interpretationinput.PreviewOutcome {
	t.Helper()
	outcome, err := previewOutcomeFromExecution(a, input, execution, runtime)
	if err != nil {
		t.Fatalf("decode preview execution: %v", err)
	}
	return outcome
}

func previewOutcomeFromExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot, execution *domainoutcome.Execution, runtime evaluationfact.RuntimeIdentity) (interpretationinput.PreviewOutcome, error) {
	ref := execution.ModelRef
	model := evaluationfact.ModelIdentity{Kind: ref.Kind(), SubKind: ref.SubKind(), Algorithm: ref.Algorithm(), Code: ref.Code().String(), Version: ref.Version(), Title: ref.Title()}
	decoded, err := evaluationfactcodec.DecodeTransientExecution(execution, model, runtime)
	if err != nil {
		return interpretationinput.PreviewOutcome{}, err
	}
	return interpretationinput.PreviewOutcome{
		Association: domainreport.Association{OrgID: a.OrgID(), AssessmentID: a.ID(), TesteeID: a.TesteeID().Uint64()},
		Input:       input, Execution: decoded, Runtime: runtime,
	}, nil
}

type charSplitPhaseReportSaver struct {
	saved bool
}

func (s *charSplitPhaseReportSaver) SaveDraft(_ *domainreport.Draft) error {
	s.saved = true
	return nil
}

type charTxRunner struct{}

func (*charTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type charEventStagerFunc func(ctx context.Context, events ...event.DomainEvent) error

func (f charEventStagerFunc) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return f(ctx, events...)
}
func mustConfiguredReportBuilder(t *testing.T) interpretationreporting.TypologyBuilder {
	t.Helper()
	return interpretationreporting.NewTypologyBuilder()
}
