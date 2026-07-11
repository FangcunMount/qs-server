package scoring_test

import (
	"context"
	"errors"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type failingAssessmentRepo struct {
	saveCalls int
	saveErr   error
}

func (r *failingAssessmentRepo) Save(_ context.Context, _ *assessment.Assessment) error {
	r.saveCalls++
	return r.saveErr
}

func (r *failingAssessmentRepo) FindByID(_ context.Context, _ assessment.ID) (*assessment.Assessment, error) {
	return nil, nil
}

func (r *failingAssessmentRepo) FindByAnswerSheetID(_ context.Context, _ assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}

func (r *failingAssessmentRepo) Delete(_ context.Context, _ assessment.ID) error {
	return nil
}

type noopScoreProjector struct{}

func (noopScoreProjector) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (noopScoreProjector) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (noopScoreProjector) Project(context.Context, evaloutcome.Outcome) error {
	return nil
}

type stubScoreProjectorRegistry struct{}

func (stubScoreProjectorRegistry) Resolve(_ evaluation.ExecutionIdentity) interpretationreporting.ScoreProjector {
	return noopScoreProjector{}
}

func (stubScoreProjectorRegistry) ResolveByMechanism(_ interpretationreporting.MechanismReportBuilderKey) interpretationreporting.ScoreProjector {
	return noopScoreProjector{}
}

func TestWriteReturnsErrorWhenAssessmentSaveFailsAfterSnapshotAndProjector(t *testing.T) {
	t.Parallel()

	a, err := assessment.NewAssessment(
		1,
		testee.NewID(9001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(8001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
		assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ID(0), meta.NewCode("SCALE-1"), "", "scale")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Submit(); err != nil {
		t.Fatal(err)
	}

	modelRef := *a.EvaluationModelRef()
	execution := assessment.NewAssessmentOutcome(
		modelRef,
		assessment.ResultSummary{PrimaryLabel: "ok"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)

	saveErr := errors.New("assessment save failed")
	repo := &failingAssessmentRepo{saveErr: saveErr}
	snapshotStore := outcomescoring.NewMemorySnapshotStore()
	writer := outcomescoring.NewWriter(repo, stubScoreProjectorRegistry{}, snapshotStore)

	err = writer.Write(context.Background(), evaloutcome.Outcome{
		Assessment: a,
		Execution:  execution,
	})
	if err == nil {
		t.Fatal("Write error = nil, want assessment save error")
	}
	if repo.saveCalls != 1 {
		t.Fatalf("assessment save calls = %d, want 1", repo.saveCalls)
	}
	if loaded, loadErr := snapshotStore.Load(context.Background(), a.ID().Uint64()); loadErr != nil || loaded == nil {
		t.Fatalf("snapshot after failed save = %#v err=%v, want persisted snapshot", loaded, loadErr)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, in-memory state should be evaluated before failed save", a.Status())
	}
}
