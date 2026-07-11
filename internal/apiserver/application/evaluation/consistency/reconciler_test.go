package consistency_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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

type stubOutcomeRepo struct {
	record *domainoutcome.Record
}

func (r stubOutcomeRepo) Save(context.Context, *domainoutcome.Record) error { return nil }
func (r stubOutcomeRepo) FindByID(context.Context, domainoutcome.ID) (*domainoutcome.Record, error) {
	return r.record, nil
}
func (r stubOutcomeRepo) FindByAssessmentID(context.Context, assessment.ID) (*domainoutcome.Record, error) {
	return r.record, nil
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
	reconciler := consistency.NewReconciler(repo, stubOutcomeChecker{exists: true}, nil, repo)

	mismatches, err := reconciler.Scan(context.Background(), []uint64{a.ID().Uint64()})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mismatches) != 1 || mismatches[0].Kind != consistency.MismatchOutcomeWithoutEvaluatedStatus {
		t.Fatalf("mismatches = %#v, want outcome_without_evaluated_status", mismatches)
	}
}

func TestRepairEvaluatedFinalizationIsIdempotent(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7005)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	execution := assessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: "ok"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)
	reconciler := consistency.NewReconciler(repo, stubOutcomeChecker{exists: true}, outcomeRecordForConsistency(t, a, execution), repo)

	if err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("RepairEvaluatedFinalization first: %v", err)
	}
	if !repo.byID[a.ID().Uint64()].Status().IsEvaluated() {
		t.Fatalf("status after repair = %s, want evaluated", repo.byID[a.ID().Uint64()].Status())
	}
	if evaluatedAt := repo.byID[a.ID().Uint64()].EvaluatedAt(); evaluatedAt == nil || !evaluatedAt.Equal(time.Unix(1, 0)) {
		t.Fatalf("evaluated_at after repair = %v, want persisted outcome time", evaluatedAt)
	}
	if err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("RepairEvaluatedFinalization second: %v", err)
	}
}

func TestRepairEvaluatedFinalizationRequiresOutcome(t *testing.T) {
	t.Parallel()

	a := submittedAssessmentForConsistency(t, 7006)
	repo := &memoryAssessmentRepo{byID: map[uint64]*assessment.Assessment{a.ID().Uint64(): a}}
	reconciler := consistency.NewReconciler(repo, stubOutcomeChecker{}, stubOutcomeRepo{}, repo)

	err := reconciler.RepairEvaluatedFinalization(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("RepairEvaluatedFinalization error = nil, want missing outcome")
	}
}

func outcomeRecordForConsistency(t *testing.T, a *assessment.Assessment, execution *assessment.AssessmentOutcome) stubOutcomeRepo {
	t.Helper()
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatal(err)
	}
	record, err := domainoutcome.NewRecord(domainoutcome.NewRecordInput{
		ID:           meta.FromUint64(8001),
		OrgID:        a.OrgID(),
		AssessmentID: a.ID(),
		TesteeID:     a.TesteeID().Uint64(),
		RunID:        "run-8001",
		Model: domainoutcome.ModelIdentity{
			Kind: modelcatalog.KindScale,
			Code: "SCALE-1",
		},
		Payload:     payload,
		EvaluatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	return stubOutcomeRepo{record: record}
}
