package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
	interpretationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// v1SplitPhaseConfig wires execute service the same way as production container assembly:
// scoringWriter -> interpretationService, optionally async with snapshot store.
type v1SplitPhaseConfig struct {
	Assessment    *assessment.Assessment
	Input         *evaluationinput.InputSnapshot
	ReportBuilder interpretationreporting.ReportBuilder

	Async          bool
	SnapshotStore  evaluationscoring.ScoringSnapshotStore
	StageEvaluated func(ctx context.Context, events ...event.DomainEvent) error
}

// buildV1SplitPhaseExecuteService mirrors container/modules/evaluation/assemble.go split-phase wiring.
// When repos are provided, the first repo is shared with the caller (cross-module harness).
func buildV1SplitPhaseExecuteService(t *testing.T, cfg v1SplitPhaseConfig, repos ...*charAssessmentRepo) (evaluationexecute.Service, *charSplitPhaseReportSaver) {
	t.Helper()

	var repo *charAssessmentRepo
	if len(repos) > 0 && repos[0] != nil {
		repo = repos[0]
	} else {
		repo = &charAssessmentRepo{assessment: cfg.Assessment}
	}
	reportSaver := &charSplitPhaseReportSaver{}
	scoreProjectors, err := interpretationreporting.NewScoreProjectorRegistry(
		interpretationreporting.NewScaleScoreProjector(&charNoopScoreRepo{}),
		interpretationreporting.NewBehavioralRatingScoreProjector(&charNoopScoreRepo{}),
		interpretationreporting.NewCognitiveScoreProjector(&charNoopScoreRepo{}),
	)
	if err != nil {
		t.Fatalf("NewScoreProjectorRegistry: %v", err)
	}
	scoringWriter := evaluationscoring.NewWriter(repo, scoreProjectors, cfg.SnapshotStore)

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

	opts := []evaluationexecute.ServiceOption{
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithScoringWriter(scoringWriter),
		evaluationexecute.WithInterpretationService(interpretationService),
	}
	if cfg.Async {
		opts = append(opts,
			evaluationexecute.WithAsyncInterpretation(true),
			evaluationexecute.WithScoringSnapshotStore(cfg.SnapshotStore),
		)
		if cfg.StageEvaluated != nil {
			opts = append(opts, evaluationexecute.WithTransactionalOutbox(
				&charTxRunner{},
				charEventStagerFunc(cfg.StageEvaluated),
			))
		}
	}

	return evaluationexecute.NewService(
		repo,
		&charInputResolver{snapshot: cfg.Input},
		opts...,
	), reportSaver
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
) (evaluationexecute.Service, *charSplitPhaseCapture) {
	t.Helper()
	capture := &charSplitPhaseCapture{}
	svc := evaluationexecute.NewService(
		&charAssessmentRepo{assessment: a},
		input,
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
		evaluationexecute.WithScoringWriter(charRecordingScoring{}),
		evaluationexecute.WithInterpretationService(&charRecordingInterpretation{cap: capture}),
	)
	return svc, capture
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
