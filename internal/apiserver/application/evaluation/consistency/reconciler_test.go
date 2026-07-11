package consistency_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
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

type stubArtifactChecker struct {
	exists bool
}

func (s stubArtifactChecker) HasScoringArtifact(context.Context, uint64) (bool, error) {
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

func TestScanDetectsScoringArtifactWithoutEvaluatedStatus(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7002)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	reconciler := consistency.NewReconciler(repo, stubArtifactChecker{exists: true}, nil, repo)

	mismatches, err := reconciler.Scan(context.Background(), []uint64{a.ID().Uint64()})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mismatches) != 1 || mismatches[0].Kind != consistency.MismatchScoringArtifactWithoutEvaluatedStatus {
		t.Fatalf("mismatches = %#v, want scoring_artifact_without_evaluated_status", mismatches)
	}
}

func TestRepairEvaluatedFinalizationIsIdempotent(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7005)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	snapshotStore := outcomescoring.NewMemorySnapshotStore()
	execution := assessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: "ok"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)
	if err := snapshotStore.Save(context.Background(), a.ID().Uint64(), execution); err != nil {
		t.Fatal(err)
	}
	reconciler := consistency.NewReconciler(repo, stubArtifactChecker{exists: true}, snapshotStore, repo)

	if err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("RepairEvaluatedFinalization first: %v", err)
	}
	if !repo.byID[a.ID().Uint64()].Status().IsEvaluated() {
		t.Fatalf("status after repair = %s, want evaluated", repo.byID[a.ID().Uint64()].Status())
	}
	if err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("RepairEvaluatedFinalization second: %v", err)
	}
}

func TestRepairEvaluatedFinalizationRequiresSnapshot(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7006)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	reconciler := consistency.NewReconciler(repo, stubArtifactChecker{exists: true}, outcomescoring.NewMemorySnapshotStore(), repo)

	err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("RepairEvaluatedFinalization error = nil, want missing snapshot")
	}
}
