package consistency_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type memoryAssessmentRepo struct {
	byID map[uint64]*assessment.Assessment
}

func (r *memoryAssessmentRepo) Save(_ context.Context, a *assessment.Assessment) error {
	if r.byID == nil {
		r.byID = make(map[uint64]*assessment.Assessment)
	}
	r.byID[a.ID().Uint64()] = a
	return nil
}

func (r *memoryAssessmentRepo) FindByID(_ context.Context, id assessment.ID) (*assessment.Assessment, error) {
	if r.byID == nil {
		return nil, nil
	}
	return r.byID[id.Uint64()], nil
}

func (r *memoryAssessmentRepo) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}

func (r *memoryAssessmentRepo) Delete(context.Context, assessment.ID) error {
	return nil
}

type stubOutcomeChecker struct {
	exists bool
}

func (s stubOutcomeChecker) HasOutcome(context.Context, uint64) (bool, error) {
	return s.exists, nil
}

func submittedAssessmentForConsistency(t *testing.T, id uint64) *assessment.Assessment {
	t.Helper()
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(9001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(8001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(id)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "", "scale")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestScanDetectsOutcomeWithoutEvaluatedStatus(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7002)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	reconciler := consistency.NewReconciler(repo, stubOutcomeChecker{exists: true})

	mismatches, err := reconciler.Scan(context.Background(), []uint64{a.ID().Uint64()})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mismatches) != 1 || mismatches[0].Kind != consistency.MismatchOutcomeWithoutEvaluatedStatus {
		t.Fatalf("mismatches = %#v, want outcome_without_evaluated_status", mismatches)
	}
}

func TestScanNeverMutatesSubmittedAssessment(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7005)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	reconciler := consistency.NewReconciler(repo, stubOutcomeChecker{exists: true})

	if _, err := reconciler.Scan(context.Background(), []uint64{a.ID().Uint64()}); err != nil {
		t.Fatal(err)
	}
	if !repo.byID[a.ID().Uint64()].Status().IsSubmitted() || repo.byID[a.ID().Uint64()].EvaluatedAt() != nil {
		t.Fatalf("scan mutated assessment: status=%s evaluated_at=%v", repo.byID[a.ID().Uint64()].Status(), repo.byID[a.ID().Uint64()].EvaluatedAt())
	}
}
