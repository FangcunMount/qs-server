package execute

import (
	"context"
	"errors"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evaluationscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type countingEvaluator struct {
	key     evaluation.EvaluatorKey
	calls   int
	outcome *domainAssessment.AssessmentOutcome
}

func (e *countingEvaluator) Key() evaluation.EvaluatorKey {
	return e.key
}

func (e *countingEvaluator) Execute(_ context.Context, _ ExecutionInput) (*domainAssessment.AssessmentOutcome, error) {
	e.calls++
	if e.outcome != nil {
		return e.outcome, nil
	}
	return domainAssessment.NewAssessmentOutcome(
		domainAssessment.NewEvaluationModelRefByCode(domainAssessment.EvaluationModelKindScale, meta.NewCode("SCALE-1"), "1.0.0", "scale"),
		domainAssessment.ResultSummary{PrimaryLabel: "recomputed"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	), nil
}

type recordingInterpretationService struct {
	calls int
}

func (s *recordingInterpretationService) GenerateAndPersist(_ context.Context, outcome evaloutcome.Outcome) error {
	s.calls++
	if outcome.Execution == nil || outcome.Execution.Summary.PrimaryLabel != "stored" {
		return errors.New("unexpected execution outcome")
	}
	return nil
}

func TestGenerateReportUsesStoredScoringSnapshotWithoutReExecute(t *testing.T) {
	t.Parallel()

	assessmentEntity, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(1001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("QNR-1"), "1.0.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithID(domainAssessment.NewID(5001)),
		domainAssessment.WithMedicalScale(domainAssessment.NewMedicalScaleRef(meta.FromUint64(3001), meta.NewCode("SCALE-1"), "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := assessmentEntity.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	modelRef := *assessmentEntity.EvaluationModelRef()
	storedOutcome := domainAssessment.NewAssessmentOutcome(
		modelRef,
		domainAssessment.ResultSummary{PrimaryLabel: "stored"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	if err := assessmentEntity.ApplyScoringOutcome(storedOutcome); err != nil {
		t.Fatalf("ApplyScoringOutcome: %v", err)
	}

	repo := &fakeAssessmentRepo{assessment: assessmentEntity}
	snapshotStore := evaluationscoring.NewMemoryScoringSnapshotStore()
	if err := snapshotStore.Save(context.Background(), assessmentEntity.ID().Uint64(), storedOutcome); err != nil {
		t.Fatalf("Save snapshot: %v", err)
	}

	evaluator := &countingEvaluator{key: evaluation.EvaluatorKeyScaleDefault}
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	interp := &recordingInterpretationService{}
	svc := NewService(
		repo,
		stubInputResolver{},
		nil,
		WithEvaluatorRegistry(registry),
		WithInterpretationService(interp),
		WithScoringSnapshotStore(snapshotStore),
	).(*service)

	if err := svc.GenerateReport(context.Background(), assessmentEntity.ID().Uint64()); err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, want 0 when snapshot exists", evaluator.calls)
	}
	if interp.calls != 1 {
		t.Fatalf("interpretation calls = %d, want 1", interp.calls)
	}
	if loaded, _ := snapshotStore.Load(context.Background(), assessmentEntity.ID().Uint64()); loaded != nil {
		t.Fatal("expected scoring snapshot to be deleted after report generation")
	}
}

type stubInputResolver struct{}

func (stubInputResolver) Resolve(_ context.Context, _ evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return &evaluationinput.InputSnapshot{}, nil
}
