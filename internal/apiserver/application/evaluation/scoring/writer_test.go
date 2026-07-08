package scoring

import (
	"context"
	"errors"
	"reflect"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type writerAssessmentRepoStub struct {
	order         *[]string
	saveCalls     int
	savedStatuses []assessment.Status
	saveErr       error
	assessment    *assessment.Assessment
}

func (r *writerAssessmentRepoStub) Save(_ context.Context, a *assessment.Assessment) error {
	if r.order != nil {
		*r.order = append(*r.order, "assessment_save")
	}
	r.saveCalls++
	r.assessment = a
	r.savedStatuses = append(r.savedStatuses, a.Status())
	return r.saveErr
}

func (r *writerAssessmentRepoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return r.assessment, nil
}

func (r *writerAssessmentRepoStub) Delete(context.Context, assessment.ID) error { return nil }

func (r *writerAssessmentRepoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}

type writerSnapshotStoreStub struct {
	order   *[]string
	saveErr error
	calls   int
}

func (s *writerSnapshotStoreStub) Save(_ context.Context, _ uint64, _ *assessment.AssessmentOutcome) error {
	if s.order != nil {
		*s.order = append(*s.order, "snapshot_save")
	}
	s.calls++
	return s.saveErr
}

func (s *writerSnapshotStoreStub) Load(context.Context, uint64) (*assessment.AssessmentOutcome, error) {
	return nil, nil
}

func (s *writerSnapshotStoreStub) Delete(context.Context, uint64) error {
	return nil
}

type writerScoreProjectorRegistryStub struct {
	projector interpretationreporting.ScoreProjector
}

func (r writerScoreProjectorRegistryStub) Resolve(evaluation.ExecutionIdentity) interpretationreporting.ScoreProjector {
	return r.projector
}

func (r writerScoreProjectorRegistryStub) ResolveByMechanism(interpretationreporting.MechanismReportBuilderKey) interpretationreporting.ScoreProjector {
	return r.projector
}

type writerScoreProjectorStub struct {
	order      *[]string
	projectErr error
	calls      int
}

func (p *writerScoreProjectorStub) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityScaleDefault
}

func (p *writerScoreProjectorStub) Key() evaluation.ExecutionIdentity {
	return p.ExecutionIdentity()
}

func (p *writerScoreProjectorStub) Project(context.Context, evaloutcome.Outcome) error {
	if p.order != nil {
		*p.order = append(*p.order, "score_project")
	}
	p.calls++
	return p.projectErr
}

func TestWriterPersistsSnapshotAndProjectionBeforeAssessmentSave(t *testing.T) {
	t.Parallel()

	var order []string
	repo := &writerAssessmentRepoStub{order: &order}
	snapshot := &writerSnapshotStoreStub{order: &order}
	projector := &writerScoreProjectorStub{order: &order}
	writer := NewWriter(repo, writerScoreProjectorRegistryStub{projector: projector}, snapshot)

	if err := writer.Write(context.Background(), scoringWriterOutcome(t)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	want := []string{"snapshot_save", "score_project", "assessment_save"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
	if len(repo.savedStatuses) != 1 || repo.savedStatuses[0] != assessment.StatusEvaluated {
		t.Fatalf("saved statuses = %#v, want evaluated", repo.savedStatuses)
	}
}

func TestWriterDoesNotSaveAssessmentWhenSnapshotFails(t *testing.T) {
	t.Parallel()

	saveErr := errors.New("snapshot unavailable")
	var order []string
	repo := &writerAssessmentRepoStub{order: &order}
	snapshot := &writerSnapshotStoreStub{order: &order, saveErr: saveErr}
	projector := &writerScoreProjectorStub{order: &order}
	writer := NewWriter(repo, writerScoreProjectorRegistryStub{projector: projector}, snapshot)

	err := writer.Write(context.Background(), scoringWriterOutcome(t))
	if !errors.Is(err, saveErr) {
		t.Fatalf("Write error = %v, want snapshot error", err)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("assessment save calls = %d, want 0", repo.saveCalls)
	}
	if projector.calls != 0 {
		t.Fatalf("projector calls = %d, want 0", projector.calls)
	}
	if want := []string{"snapshot_save"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func TestWriterDoesNotSaveAssessmentWhenProjectorFails(t *testing.T) {
	t.Parallel()

	projectErr := errors.New("project failed")
	var order []string
	repo := &writerAssessmentRepoStub{order: &order}
	snapshot := &writerSnapshotStoreStub{order: &order}
	projector := &writerScoreProjectorStub{order: &order, projectErr: projectErr}
	writer := NewWriter(repo, writerScoreProjectorRegistryStub{projector: projector}, snapshot)

	err := writer.Write(context.Background(), scoringWriterOutcome(t))
	if !errors.Is(err, projectErr) {
		t.Fatalf("Write error = %v, want projector error", err)
	}
	if repo.saveCalls != 0 {
		t.Fatalf("assessment save calls = %d, want 0", repo.saveCalls)
	}
	if want := []string{"snapshot_save", "score_project"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %#v, want %#v", order, want)
	}
}

func scoringWriterOutcome(t *testing.T) evaloutcome.Outcome {
	t.Helper()
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(1001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(2001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(3001)),
		assessment.WithMedicalScale(assessment.NewMedicalScaleRef(meta.FromUint64(4001), meta.NewCode("SCALE-1"), "scale")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	execution := assessment.NewAssessmentOutcome(
		*a.EvaluationModelRef(),
		assessment.ResultSummary{PrimaryLabel: "ok"},
		assessment.EvaluationDetail{Kind: assessment.EvaluationModelKindScale},
	)
	return evaloutcome.Outcome{Assessment: a, Execution: execution}
}

var (
	_ assessment.Repository                          = (*writerAssessmentRepoStub)(nil)
	_ ScoringSnapshotStore                           = (*writerSnapshotStoreStub)(nil)
	_ interpretationreporting.ScoreProjector         = (*writerScoreProjectorStub)(nil)
	_ interpretationreporting.ScoreProjectorRegistry = writerScoreProjectorRegistryStub{}
)
