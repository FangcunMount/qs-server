package assessment

import (
	"context"
	"fmt"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// intakeAssessmentRepo characterizes the answer-sheet orchestration contract:
// creation must persist an Assessment before its later submit transition.
type intakeAssessmentRepo struct {
	assessment *domainassessment.Assessment
	saves      int
}

func (r *intakeAssessmentRepo) Save(_ context.Context, assessment *domainassessment.Assessment) error {
	if assessment == nil {
		return fmt.Errorf("assessment is required")
	}
	if assessment.ID().IsZero() {
		assessment.AssignID(domainassessment.NewID(7001))
	}
	r.assessment = assessment
	r.saves++
	return nil
}

func (r *intakeAssessmentRepo) FindByID(_ context.Context, id domainassessment.ID) (*domainassessment.Assessment, error) {
	if r.assessment == nil || r.assessment.ID() != id {
		return nil, fmt.Errorf("assessment not found")
	}
	return r.assessment, nil
}

func (*intakeAssessmentRepo) FindByAnswerSheetID(context.Context, domainassessment.AnswerSheetRef) (*domainassessment.Assessment, error) {
	return nil, fmt.Errorf("assessment not found")
}

func (*intakeAssessmentRepo) Delete(context.Context, domainassessment.ID) error { return nil }

func TestAnswerSheetIntakeCreateThenSubmitPersistsAndStagesEvaluationRequest(t *testing.T) {
	repo := &intakeAssessmentRepo{}
	txRunner := &recordingTxRunner{}
	stager := &recordingEventStager{}
	service := NewAnswerSheetAssessmentIntakeService(
		repo,
		domainassessment.NewDefaultAssessmentCreator(),
		txRunner,
		stager,
		nil,
	)

	created, err := service.CreateForAnswerSheet(context.Background(), CreateAssessmentDTO{
		OrgID:                1,
		TesteeID:             2,
		QuestionnaireCode:    "Q-001",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        3,
		OriginType:           "adhoc",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID != 7001 || created.Status != domainassessment.StatusPending.String() {
		t.Fatalf("created assessment = %#v", created)
	}

	submitted, err := service.SubmitForEvaluation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if submitted.Status != domainassessment.StatusSubmitted.String() {
		t.Fatalf("submitted status = %q, want %q", submitted.Status, domainassessment.StatusSubmitted)
	}
	if !txRunner.called || repo.saves != 2 {
		t.Fatalf("transaction/save contract = tx:%v saves:%d, want true/2", txRunner.called, repo.saves)
	}
	if len(stager.eventTypes) == 0 || stager.eventTypes[len(stager.eventTypes)-1] != domainassessment.EventTypeRequested {
		t.Fatalf("staged events = %#v, want final %q", stager.eventTypes, domainassessment.EventTypeRequested)
	}
}

func TestTesteeAssessmentQueryRejectsAnotherTesteesAssessment(t *testing.T) {
	owner := testee.NewID(22)
	assessment, err := domainassessment.NewAssessment(
		1,
		owner,
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "v1"),
		domainassessment.NewAnswerSheetRef(meta.FromUint64(3)),
		domainassessment.NewAdhocOrigin(),
		domainassessment.WithID(domainassessment.NewID(7002)),
	)
	if err != nil {
		t.Fatal(err)
	}
	service := NewTesteeAssessmentQueryService(
		&intakeAssessmentRepo{assessment: assessment},
		nil,
		nil,
	)

	if _, err := service.GetMine(context.Background(), 23, assessment.ID().Uint64()); err == nil {
		t.Fatal("GetMine must reject a different testee")
	}
}

func TestWorkerAssessmentResultReaderUsesDedicatedTrustedReadPort(t *testing.T) {
	owner := testee.NewID(22)
	a, err := domainassessment.NewAssessment(
		1,
		owner,
		domainassessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "v1"),
		domainassessment.NewAnswerSheetRef(meta.FromUint64(3)),
		domainassessment.NewAdhocOrigin(),
		domainassessment.WithID(domainassessment.NewID(7003)),
	)
	if err != nil {
		t.Fatal(err)
	}

	reader := NewWorkerAssessmentResultReader(&intakeAssessmentRepo{assessment: a})
	result, err := reader.GetByID(context.Background(), a.ID().Uint64())
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if result.ID != a.ID().Uint64() || result.TesteeID != owner.Uint64() {
		t.Fatalf("worker result = %#v", result)
	}
}

var _ domainassessment.Repository = (*intakeAssessmentRepo)(nil)
var _ AnswerSheetAssessmentIntakeService = (*assessmentIntakeService)(nil)
var _ TesteeAssessmentQueryService = (*testeeAssessmentQueryService)(nil)
var _ AssessmentResultReader = (*workerAssessmentResultReader)(nil)
