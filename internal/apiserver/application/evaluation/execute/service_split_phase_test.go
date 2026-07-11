package execute

import (
	"context"
	"sync"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type phaseRecorder struct {
	mu     sync.Mutex
	phases []string
}

func (r *phaseRecorder) record(phase string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phases = append(r.phases, phase)
}

func (r *phaseRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.phases))
	copy(out, r.phases)
	return out
}

type phaseRecordingScoringWriter struct {
	rec *phaseRecorder
}

func (w *phaseRecordingScoringWriter) Write(_ context.Context, outcome evaloutcome.Outcome) error {
	w.rec.record("scoring")
	if outcome.Assessment == nil || outcome.Execution == nil {
		return nil
	}
	return outcome.Assessment.ApplyScoringOutcome(outcome.Execution)
}

type phaseRecordingInterpretationService struct {
	rec   *phaseRecorder
	calls int
}

func (s *phaseRecordingInterpretationService) GenerateAndPersist(context.Context, evaloutcome.Outcome) error {
	s.calls++
	s.rec.record("interpretation")
	return nil
}

func TestEvaluateSyncSplitPhaseInvokesScoringBeforeInterpretation(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	rec := &phaseRecorder{}
	scoring := &phaseRecordingScoringWriter{rec: rec}
	interp := &phaseRecordingInterpretationService{rec: rec}

	evaluator := &countingEvaluator{
		key: evaluation.ExecutionIdentityScaleDefault,
		outcome: domainAssessment.NewAssessmentOutcome(
			*a.EvaluationModelRef(),
			domainAssessment.ResultSummary{PrimaryLabel: "ok"},
			domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
		),
	}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}

	svc := NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithScoringWriter(scoring),
		WithInterpretationService(interp),
	)
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := rec.snapshot(); len(got) != 2 || got[0] != "scoring" || got[1] != "interpretation" {
		t.Fatalf("phase order = %#v, want [scoring interpretation]", got)
	}
}

func TestEvaluateRequiresSplitPhaseWriters(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}

	svc := NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
	)
	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("Evaluate error = nil, want split-phase configuration error")
	}
}

func TestEvaluateAsyncSplitPhaseStagesEvaluatedWithoutInterpretation(t *testing.T) {
	t.Parallel()

	a := splitPhaseAssessment(t)
	rec := &phaseRecorder{}
	scoring := &phaseRecordingScoringWriter{rec: rec}
	interp := &phaseRecordingInterpretationService{rec: rec}
	stager := &engineRecordingEventStager{}
	snapshotStore := outcomescoring.NewMemorySnapshotStore()

	evaluator := &countingEvaluator{
		key: evaluation.ExecutionIdentityScaleDefault,
		outcome: domainAssessment.NewAssessmentOutcome(
			*a.EvaluationModelRef(),
			domainAssessment.ResultSummary{PrimaryLabel: "stored"},
			domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
		),
	}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}

	svc := NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithScoringWriter(scoring),
		WithInterpretationService(interp),
		WithAsyncInterpretation(true),
		WithScoringSnapshotStore(snapshotStore),
		WithTransactionalOutbox(&engineRecordingTxRunner{}, stager),
	)
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := rec.snapshot(); len(got) != 1 || got[0] != "scoring" {
		t.Fatalf("phase order = %#v, want [scoring]", got)
	}
	if interp.calls != 0 {
		t.Fatalf("interpretation calls = %d, want 0 during async Evaluate", interp.calls)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", a.Status())
	}
	if len(stager.eventTypes) == 0 || stager.eventTypes[len(stager.eventTypes)-1] != eventcatalog.AssessmentEvaluated {
		t.Fatalf("staged events = %#v, want terminal %q", stager.eventTypes, eventcatalog.AssessmentEvaluated)
	}
}

func splitPhaseAssessment(t *testing.T) *domainAssessment.Assessment {
	t.Helper()
	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(9001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(8001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithID(domainAssessment.NewID(7001)),
		domainAssessment.WithEvaluationModel(domainAssessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "", "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return a
}

var _ outcomescoring.Writer = (*phaseRecordingScoringWriter)(nil)
