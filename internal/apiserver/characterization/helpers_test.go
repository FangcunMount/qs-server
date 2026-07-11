package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"

	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// v1SplitPhaseConfig preserves the legacy characterization fixtures while keeping
// Evaluation and Interpretation as two explicit test-side use-case calls.
type v1SplitPhaseConfig struct {
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	ReportBuilder interpretationreporting.ReportBuilder

	Async          bool
	SnapshotStore  outcomescoring.SnapshotStore
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
	scoreProjectors, err := interpretationreporting.NewScoreProjectorRegistry(
		interpretationreporting.NewFactorScoringScoreProjector(&charNoopScoreRepo{}),
		interpretationreporting.NewNormProfileScoreProjector(&charNoopScoreRepo{}),
		interpretationreporting.NewTaskPerformanceScoreProjector(&charNoopScoreRepo{}),
	)
	if err != nil {
		t.Fatalf("NewScoreProjectorRegistry: %v", err)
	}
	scoringWriter := &charCapturingScoringWriter{delegate: outcomescoring.NewWriter(repo, scoreProjectors, cfg.SnapshotStore)}

	reportBuilders, err := interpretationreporting.NewReportBuilderRegistry(cfg.ReportBuilder)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	interpretationWriter, err := interpretationreporting.NewInterpretationWriter(
		repo,
		scoreProjectors,
		reportBuilders,
		reportSaver,
		&charNoopCompletionNotifier{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewInterpretationWriter: %v", err)
	}
	interpretationService := interpretationapp.NewService(interpretationWriter)

	runtimeDescriptorRegistry := wireV1RuntimeDescriptorRegistry(t)
	familyEvaluators := newV1FamilyEvaluators(t)

	opts := []evaluationexecute.ServiceOption{
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithRuntimeDescriptorRegistry(runtimeDescriptorRegistry),
		evaluationexecute.WithFamilyEvaluators(familyEvaluators),
		evaluationexecute.WithScoringWriter(scoringWriter),
		evaluationexecute.WithTransactionalOutbox(&charTxRunner{}, charEventStagerFunc(func(ctx context.Context, events ...event.DomainEvent) error {
			if cfg.StageEvaluated != nil {
				return cfg.StageEvaluated(ctx, events...)
			}
			return nil
		})),
	}

	core := evaluationexecute.NewService(
		repo,
		&charInputResolver{snapshot: cfg.Input},
		opts...,
	)
	return &charSplitPhaseService{
		Service:        core,
		interpretation: interpretationService,
		capture:        scoringWriter,
		snapshotStore:  cfg.SnapshotStore,
		inlineLegacy:   !cfg.Async,
	}, reportSaver
}

type charCapturingScoringWriter struct {
	delegate outcomescoring.Writer
	outcome  evaloutcome.Outcome
}

func (w *charCapturingScoringWriter) Write(ctx context.Context, outcome evaloutcome.Outcome) error {
	w.outcome = outcome
	return w.delegate.Write(ctx, outcome)
}

type charSplitPhaseService struct {
	evaluationexecute.Service
	interpretation interpretationapp.Service
	capture        *charCapturingScoringWriter
	snapshotStore  outcomescoring.SnapshotStore
	inlineLegacy   bool
}

func (s *charSplitPhaseService) Evaluate(ctx context.Context, assessmentID uint64) error {
	if err := s.Service.Evaluate(ctx, assessmentID); err != nil {
		return err
	}
	if s.inlineLegacy {
		return s.GenerateReport(ctx, assessmentID)
	}
	return nil
}

func (s *charSplitPhaseService) GenerateReport(ctx context.Context, assessmentID uint64) error {
	if err := s.interpretation.GenerateAndPersist(ctx, s.capture.outcome); err != nil {
		return err
	}
	if s.snapshotStore != nil {
		_ = s.snapshotStore.Delete(ctx, assessmentID)
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

type charRecordingScoring struct{}

func (charRecordingScoring) Write(_ context.Context, outcome evaloutcome.Outcome) error {
	if outcome.Assessment != nil && outcome.Execution != nil {
		return outcome.Assessment.ApplyScoringOutcome(outcome.Execution)
	}
	return nil
}

func newV1RecordingExecuteService(
	t *testing.T,
	a *assessment.Assessment,
	input *charInputResolver,
) (*charSplitPhaseService, *charSplitPhaseCapture) {
	t.Helper()
	capture := &charSplitPhaseCapture{}
	recording := &charCapturingScoringWriter{delegate: charRecordingScoring{}}
	core := evaluationexecute.NewService(
		&charAssessmentRepo{assessment: a},
		input,
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithRuntimeDescriptorRegistry(wireV1RuntimeDescriptorRegistry(t)),
		evaluationexecute.WithFamilyEvaluators(newV1FamilyEvaluators(t)),
		evaluationexecute.WithScoringWriter(recording),
		evaluationexecute.WithTransactionalOutbox(&charTxRunner{}, charEventStagerFunc(func(context.Context, ...event.DomainEvent) error { return nil })),
	)
	return &charSplitPhaseService{
		Service:        core,
		interpretation: &charRecordingInterpretation{cap: capture},
		capture:        recording,
		inlineLegacy:   true,
	}, capture
}

type charNoopScoreRepo struct{}

func (*charNoopScoreRepo) SaveScoresWithContext(context.Context, *assessment.Assessment, *assessment.ScaleScoreProjection) error {
	return nil
}
func (*charNoopScoreRepo) DeleteByAssessmentID(context.Context, assessment.ID) error { return nil }

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

type charNoopCompletionNotifier struct{}

func (*charNoopCompletionNotifier) NotifyCompletion(context.Context, evaloutcome.Outcome) {}

type charTxRunner struct{}

func (*charTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type charEventStagerFunc func(ctx context.Context, events ...event.DomainEvent) error

func (f charEventStagerFunc) Stage(ctx context.Context, events ...event.DomainEvent) error {
	return f(ctx, events...)
}
func mustConfiguredReportBuilder(t *testing.T) typologyeval.ReportBuilder {
	t.Helper()
	builder, err := typologyeval.NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	return builder
}
