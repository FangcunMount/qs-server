package evaluation

import (
	"context"
	"database/sql"
	"testing"

	"github.com/FangcunMount/component-base/pkg/event"
	evaluationintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	_ "github.com/go-sql-driver/mysql"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestNewAssessmentRepositoryCreatesCommandRepository(t *testing.T) {
	repo, ok := NewAssessmentRepository(nil).(*assessmentRepository)
	if !ok {
		t.Fatalf("repository type = %T, want *assessmentRepository", repo)
	}
	if repo.mapper == nil {
		t.Fatalf("mapper = nil")
	}
}

// TestCreateForAnswerSheetThenSubmitForEvaluationWithConcreteRepository
// covers the production create-then-submit service sequence while delegating
// both writes to the concrete MySQL repository. A dry-run DB keeps the test
// hermetic; the lookup wrapper only provides the just-persisted aggregate for
// SubmitForEvaluation's repository read.
type acceptingModelValidator struct{}

func (acceptingModelValidator) ValidateEvaluationModel(context.Context, domainassessment.EvaluationModelRef, domainassessment.QuestionnaireRef, evaluationintake.ModelValidationMode) error {
	return nil
}

func TestCreateForAnswerSheetThenSubmitForEvaluationWithConcreteRepository(t *testing.T) {
	repo := &persistedAssessmentRepository{Repository: NewAssessmentRepository(newDryRunAssessmentDB(t))}
	service := evaluationintake.NewService(repo, acceptingModelValidator{}, immediateTransactionRunner{}, discardEventStager{}, nil)

	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := service.CreateForAnswerSheet(context.Background(), evaluationintake.CreateCommand{
		OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", AnswerSheetID: 3, OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	if err != nil {
		t.Fatalf("CreateForAnswerSheet: %v", err)
	}
	if created.ID == 0 || created.Status != "pending" {
		t.Fatalf("created assessment = %#v", created)
	}

	submitted, err := service.SubmitForEvaluation(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("SubmitForEvaluation: %v", err)
	}
	if submitted.ID != created.ID || submitted.Status != "submitted" {
		t.Fatalf("submitted assessment = %#v", submitted)
	}
}

type persistedAssessmentRepository struct {
	domainassessment.Repository
	item *domainassessment.Assessment
}

func (r *persistedAssessmentRepository) Save(ctx context.Context, item *domainassessment.Assessment) error {
	if err := r.Repository.Save(ctx, item); err != nil {
		return err
	}
	r.item = item
	return nil
}

func (r *persistedAssessmentRepository) FindByID(ctx context.Context, id domainassessment.ID) (*domainassessment.Assessment, error) {
	if r.item != nil && r.item.ID() == id {
		return r.item, nil
	}
	return r.Repository.FindByID(ctx, id)
}

type immediateTransactionRunner struct{}

func (immediateTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type discardEventStager struct{}

func (discardEventStager) Stage(context.Context, ...event.DomainEvent) error { return nil }

func newDryRunAssessmentDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := sql.Open("mysql", "user:pass@tcp(127.0.0.1:3306)/qs_server_dry_run?charset=utf8mb4&parseTime=True&loc=Local")
	if err != nil {
		t.Fatalf("open dry-run sql db: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn:                      conn,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("open dry-run gorm db: %v", err)
	}
	return db
}
