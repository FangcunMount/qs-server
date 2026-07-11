//go:build refactor_target

package execute

import (
	"context"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var errTargetReportGeneration = errors.New("target report generation failed")

type failThenSucceedInterpretationService struct {
	calls int
}

func (s *failThenSucceedInterpretationService) GenerateAndPersist(_ context.Context, _ evaloutcome.Outcome) error {
	s.calls++
	if s.calls == 1 {
		return errTargetReportGeneration
	}
	return nil
}

func TestTargetReportFailureDoesNotModifyEvaluationFacts(t *testing.T) {
	t.Parallel()

	a, outcome := targetEvaluatedAssessmentForReport(t)
	snapshotStore := outcomescoring.NewMemorySnapshotStore()
	if err := snapshotStore.Save(context.Background(), a.ID().Uint64(), outcome); err != nil {
		t.Fatalf("Save snapshot: %v", err)
	}
	finishedAt := time.Now()
	succeededRun := evalrun.EvaluationRun{
		RunID:        "5001:1",
		AssessmentID: a.ID().Uint64(),
		Attempt:      evalrun.Attempt{Number: 1, Status: evalrun.StatusSucceeded},
		FinishedAt:   &finishedAt,
	}
	runRepo := &stubRunRepo{latest: &succeededRun}
	interp := &failThenSucceedInterpretationService{}
	svc := targetReportService(t, a, snapshotStore, runRepo, interp, &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault})
	beforeScore := *a.TotalScore()

	err := svc.GenerateReport(context.Background(), a.ID().Uint64())
	if !errors.Is(err, errTargetReportGeneration) {
		t.Fatalf("GenerateReport error = %v, want report failure", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, report failure must preserve evaluated", a.Status())
	}
	if a.TotalScore() == nil || *a.TotalScore() != beforeScore {
		t.Fatalf("assessment score = %v, want preserved %v", a.TotalScore(), beforeScore)
	}
	if runRepo.latest.Attempt.Status != evalrun.StatusSucceeded || len(runRepo.saved) != 0 {
		t.Fatalf("evaluation run changed after report failure: latest=%s saved=%#v", runRepo.latest.Attempt.Status, runRepo.saved)
	}
	if stored, loadErr := snapshotStore.Load(context.Background(), a.ID().Uint64()); loadErr != nil || stored == nil {
		t.Fatalf("evaluation outcome disappeared after report failure: outcome=%#v err=%v", stored, loadErr)
	}
}

func TestTargetReportRetryDoesNotExecuteEvaluator(t *testing.T) {
	t.Parallel()

	a, outcome := targetEvaluatedAssessmentForReport(t)
	snapshotStore := outcomescoring.NewMemorySnapshotStore()
	if err := snapshotStore.Save(context.Background(), a.ID().Uint64(), outcome); err != nil {
		t.Fatalf("Save snapshot: %v", err)
	}
	evaluator := &countingEvaluator{key: evaluation.ExecutionIdentityScaleDefault}
	interp := &failThenSucceedInterpretationService{}
	svc := targetReportService(t, a, snapshotStore, &stubRunRepo{}, interp, evaluator)

	if err := svc.GenerateReport(context.Background(), a.ID().Uint64()); !errors.Is(err, errTargetReportGeneration) {
		t.Fatalf("first GenerateReport error = %v, want report failure", err)
	}
	if err := svc.GenerateReport(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("retry GenerateReport: %v", err)
	}
	if interp.calls != 2 {
		t.Fatalf("interpretation calls = %d, want initial attempt and retry", interp.calls)
	}
	if evaluator.calls != 0 {
		t.Fatalf("evaluator calls = %d, report retry must reuse persisted evaluation outcome", evaluator.calls)
	}
}

func targetReportService(
	t *testing.T,
	a *domainAssessment.Assessment,
	snapshotStore outcomescoring.SnapshotStore,
	runRepo *stubRunRepo,
	interp *failThenSucceedInterpretationService,
	evaluator *countingEvaluator,
) *service {
	t.Helper()
	registry, err := NewEvaluatorRegistry(evaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	return NewService(
		&fakeAssessmentRepo{assessment: a},
		stubInputResolver{},
		WithEvaluatorRegistry(registry),
		WithRunRepository(runRepo),
		WithInterpretationService(interp),
		WithScoringSnapshotStore(snapshotStore),
	).(*service)
}

func targetEvaluatedAssessmentForReport(t *testing.T) (*domainAssessment.Assessment, *domainAssessment.AssessmentOutcome) {
	t.Helper()
	a, err := domainAssessment.NewAssessment(
		1,
		testee.NewID(1001),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-TARGET"), "1.0.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithID(domainAssessment.NewID(5001)),
		domainAssessment.WithEvaluationModel(domainAssessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "1.0.0", "target scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	outcome := domainAssessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		domainAssessment.ResultSummary{PrimaryLabel: "stored"},
		domainAssessment.EvaluationDetail{Kind: domainAssessment.EvaluationModelKindScale},
	)
	outcome.Primary = &domainAssessment.OutcomeScoreValue{Kind: domainAssessment.OutcomeScoreKindRawTotal, Value: 12}
	if err := a.ApplyScoringOutcome(outcome); err != nil {
		t.Fatalf("ApplyScoringOutcome: %v", err)
	}
	return a, outcome
}
