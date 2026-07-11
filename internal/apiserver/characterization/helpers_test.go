package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// v1SplitPhaseConfig preserves the legacy characterization fixtures while keeping
// Evaluation and Interpretation as two explicit test-side use-case calls.
type v1SplitPhaseConfig struct {
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	ReportBuilder interpretationreporting.ReportBuilder

	Async          bool
	StageEvaluated func(ctx context.Context, events ...event.DomainEvent) error
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

	reportBuilders, err := interpretationreporting.NewReportBuilderRegistry(cfg.ReportBuilder)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	reportGenerator, err := interpretationreporting.NewGenerator(reportBuilders)
	if err != nil {
		t.Fatalf("NewGenerator: %v", err)
	}

	runtimeDescriptorRegistry := wireV1RuntimeDescriptorRegistry(t)
	familyEvaluators := newV1FamilyEvaluators(t)

	opts := []evaluationexecute.EngineOption{
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithRuntimeDescriptorRegistry(runtimeDescriptorRegistry),
		evaluationexecute.WithFamilyEvaluators(familyEvaluators),
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
		generateReport: func(ctx context.Context, outcome evaloutcome.Outcome) error {
			generation, err := reportGenerator.Generate(ctx, outcome)
			if err != nil {
				return err
			}
			return reportSaver.SaveReportDurably(ctx, generation.Report, outcome.TesteeID(), generation.Events)
		},
	}, reportSaver
}

type charCapturingEvaluationCommitter struct {
	stage   func(ctx context.Context, events ...event.DomainEvent) error
	outcome evaloutcome.Outcome
}

func (c *charCapturingEvaluationCommitter) Commit(ctx context.Context, request outcomecommit.Request) (*domainoutcome.Record, error) {
	c.outcome = request.Outcome
	if request.Outcome.Assessment == nil || request.Outcome.Execution == nil {
		return nil, nil
	}
	if err := request.Outcome.Assessment.ApplyScoringProjection(evaloutcome.ScoringProjectionFromExecution(request.Outcome.Execution)); err != nil {
		return nil, err
	}
	if request.Run != nil {
		if err := request.Run.Succeed(request.EvaluatedAt); err != nil {
			return nil, err
		}
		request.Outcome.Assessment.StageEvaluatedEvent(request.EvaluatedAt, meta.FromUint64(1), request.Run.RunID)
	}
	if c.stage != nil {
		events := request.Outcome.Assessment.Events()
		if err := c.stage(ctx, events...); err != nil {
			return nil, err
		}
	}
	request.Outcome.Assessment.ClearEvents()
	return nil, nil
}

type charSplitPhaseService struct {
	evaluationexecute.Engine
	capture        *charCapturingEvaluationCommitter
	inlineLegacy   bool
	generateReport func(context.Context, evaloutcome.Outcome) error
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
	if run.Attempt.Status == evalrun.StatusRunning && run.HasActiveLease(request.ClaimedAt) {
		return evaluationrun.ClaimResult{Run: run}, nil
	}
	if err := run.Claim(request.Token, request.ClaimedAt, request.LeaseUntil); err != nil {
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
	outcome             evaloutcome.Outcome
}

type charRecordingInterpretation struct {
	cap *charSplitPhaseCapture
}

func (s *charRecordingInterpretation) GenerateAndPersist(_ context.Context, outcome evaloutcome.Outcome) error {
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
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithRuntimeDescriptorRegistry(wireV1RuntimeDescriptorRegistry(t)),
		evaluationexecute.WithFamilyEvaluators(newV1FamilyEvaluators(t)),
		evaluationexecute.WithEvaluationCommitter(recording),
		evaluationexecute.WithRunRepository(&charRunRepo{}),
		evaluationexecute.WithTransactionalOutbox(&charTxRunner{}, charEventStagerFunc(func(context.Context, ...event.DomainEvent) error { return nil })),
	)
	return &charSplitPhaseService{
		Engine:       core,
		capture:      recording,
		inlineLegacy: true,
		generateReport: func(ctx context.Context, outcome evaloutcome.Outcome) error {
			return (&charRecordingInterpretation{cap: capture}).GenerateAndPersist(ctx, outcome)
		},
	}, capture
}

type charSplitPhaseReportSaver struct {
	saved      bool
	eventTypes []string
}

func (s *charSplitPhaseReportSaver) SaveReportDurably(_ context.Context, _ *domainreport.InterpretReport, _ testee.ID, events []event.DomainEvent) error {
	s.saved = true
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
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
func mustConfiguredReportBuilder(t *testing.T) typologyreporting.ReportBuilder {
	t.Helper()
	builder, err := typologyreporting.NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	return builder
}
