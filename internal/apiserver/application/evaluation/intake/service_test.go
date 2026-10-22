package intake

import (
	"context"
	"errors"
	"testing"
	"time"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
)

type intakeRepoStub struct {
	aggregate *domainassessment.Assessment
	saves     int
}

func (r *intakeRepoStub) Save(_ context.Context, a *domainassessment.Assessment) error {
	if a.ID().IsZero() {
		a.AssignID(domainassessment.NewID(7001))
	}
	r.aggregate = a
	r.saves++
	return nil
}
func (r *intakeRepoStub) FindByID(_ context.Context, id domainassessment.ID) (*domainassessment.Assessment, error) {
	if r.aggregate == nil || r.aggregate.ID() != id {
		return nil, errors.New("not found")
	}
	return r.aggregate, nil
}
func (r *intakeRepoStub) FindByAnswerSheetID(_ context.Context, ref domainassessment.AnswerSheetRef) (*domainassessment.Assessment, error) {
	if r.aggregate == nil || r.aggregate.AnswerSheetRef() != ref {
		return nil, errors.New("not found")
	}
	return r.aggregate, nil
}
func (*intakeRepoStub) Delete(context.Context, domainassessment.ID) error { return nil }

type txStub struct{ calls int }

func (t *txStub) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.calls++
	return fn(ctx)
}

type stagerStub struct{ types []string }

func (s *stagerStub) Stage(_ context.Context, events ...event.DomainEvent) error {
	for _, item := range events {
		s.types = append(s.types, item.EventType())
	}
	return nil
}

type validatorStub struct{}

func (validatorStub) ValidateEvaluationModel(context.Context, domainassessment.EvaluationModelRef, domainassessment.QuestionnaireRef) error {
	return nil
}

func TestServiceCreatesThenSubmitsAssessmentThroughTransactionalOutbox(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	service := NewService(repo, validatorStub{}, tx, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := service.CreateForAnswerSheet(context.Background(), CreateCommand{
		OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", AnswerSheetID: 3, OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 7001 || created.Status != "pending" {
		t.Fatalf("created=%#v", created)
	}
	submitted, err := service.SubmitForEvaluation(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if submitted.Status != "submitted" || repo.saves != 2 || tx.calls != 2 {
		t.Fatalf("submitted=%#v saves=%d tx=%d", submitted, repo.saves, tx.calls)
	}
	found := false
	for _, eventType := range stager.types {
		if eventType == domainassessment.EventTypeRequested {
			found = true
		}
	}
	if !found {
		t.Fatalf("staged=%v", stager.types)
	}
}

func TestServiceRejectsSubmitForEvaluationWithoutModel(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	service := NewService(repo, nil, tx, stager)
	created, err := service.CreateForAnswerSheet(context.Background(), CreateCommand{
		OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", AnswerSheetID: 3, OriginType: "adhoc",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SubmitForEvaluation(context.Background(), created.ID); err == nil {
		t.Fatal("expected SubmitForEvaluation without model to fail")
	}
	if repo.saves != 1 || repo.aggregate == nil || !repo.aggregate.Status().IsPending() {
		t.Fatalf("unbound submit mutated durable state: saves=%d status=%v", repo.saves, repo.aggregate)
	}
}

func TestServiceRejectsBoundModelWhenValidatorIsMissing(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	modelCode := "MODEL-1"
	service := NewService(repo, nil, tx, stager)
	if _, err := service.CreateForAnswerSheet(context.Background(), CreateCommand{
		OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", AnswerSheetID: 3,
		ModelCode: &modelCode, OriginType: "adhoc",
	}); err == nil {
		t.Fatal("expected missing evaluation model validator error")
	}
	if repo.saves != 0 || tx.calls != 0 || len(stager.types) != 0 {
		t.Fatalf("invalid configuration produced side effects: saves=%d tx=%d events=%v", repo.saves, tx.calls, stager.types)
	}
}

var _ domainassessment.Repository = (*intakeRepoStub)(nil)

// This application stub does not prove concurrency; the real MySQL test does.
func (r *intakeRepoStub) SavePendingSubmission(ctx context.Context, a *domainassessment.Assessment) error {
	return r.Save(ctx, a)
}

// A repository decorator that drops the atomic capability must fail closed.
type proofPlainRepository struct{ domainassessment.Repository }

func TestSubmissionRejectsRepositoryWithoutAtomicCapability(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	service := NewService(proofPlainRepository{Repository: repo}, validatorStub{}, tx, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := service.CreateForAnswerSheet(context.Background(), CreateCommand{
		OrgID: 1, TesteeID: 2, AnswerSheetID: 3, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
		ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(stager.types)
	if _, err := service.SubmitForEvaluation(context.Background(), created.ID); err == nil {
		t.Fatal("unsafe submission accepted")
	}
	if repo.saves != 1 || tx.calls != 1 || len(stager.types) != beforeEvents {
		t.Fatal("unsupported submission fell back to ordinary Save")
	}
}

type transactionSubmitStub struct {
	called bool
	err    error
}

func (s *transactionSubmitStub) Submit(_ context.Context, a *domainassessment.Assessment, _ time.Time) (*domainassessment.Assessment, error) {
	s.called = true
	if !a.Status().IsPending() {
		return nil, errors.New("caller mutated before transaction submission")
	}
	return nil, s.err
}
func TestServiceUsesExplicitTransactionSubmitterWithoutLegacyFallback(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	unknown := eventtransaction.ErrUnresolved
	submit := &transactionSubmitStub{err: unknown}
	svc := NewService(repo, validatorStub{}, tx, stager, WithTransactionSubmitter(submit))
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	created, err := svc.CreateForAnswerSheet(t.Context(), CreateCommand{OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q", QuestionnaireVersion: "1", AnswerSheetID: 3, OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.SubmitForEvaluation(t.Context(), created.ID)
	if !errors.Is(err, unknown) || result != nil || !submit.called {
		t.Fatalf("result=%v error=%v called=%v", result, err, submit.called)
	}
	if repo.saves != 1 || tx.calls != 1 || len(stager.types) != 0 || !repo.aggregate.Status().IsPending() {
		t.Fatal("transaction unknown fell back to legacy staging or mutated caller")
	}
}

func TestFinalTransactionModeCreatesEventlessPendingWithoutLegacyStager(t *testing.T) {
	repo, tx := &intakeRepoStub{}, &txStub{}
	submit := &transactionSubmitStub{err: eventtransaction.ErrUnresolved}
	svc := NewService(repo, validatorStub{}, tx, nil, WithTransactionSubmitter(submit))
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	pending, err := svc.CreateForAnswerSheet(t.Context(), CreateCommand{OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q", QuestionnaireVersion: "1", AnswerSheetID: 3, OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version})
	if err != nil || pending == nil || pending.Status != "pending" {
		t.Fatal("final host required publication Outbox for an eventless draft", err)
	}
	if tx.calls != 1 || repo.saves != 1 || submit.called || len(repo.aggregate.Events()) != 0 {
		t.Fatal("pending creation sent messages or lost original local transaction")
	}
	// Event-bearing submission must still go exclusively through the explicit
	// sender and retain Unknown; it must never use the eventless write path.
	result, err := svc.SubmitForEvaluation(t.Context(), pending.ID)
	if !errors.Is(err, eventtransaction.ErrUnresolved) || result != nil || !submit.called || repo.saves != 1 {
		t.Fatal("submission fell back to a local draft write", err)
	}
	if err := repo.aggregate.SubmitAt(time.Now()); err != nil {
		t.Fatal(err)
	}
	before := tx.calls
	if err := (assessmentCreateFinalizer{repo: repo, txRunner: tx}).SavePending(t.Context(), repo.aggregate); err == nil || tx.calls != before {
		t.Fatal("event-bearing fact passed through eventless draft path")
	}
}
