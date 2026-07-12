package intake

import (
	"context"
	"errors"
	"testing"

	domainassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
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

func TestServiceCreatesThenSubmitsAssessmentThroughTransactionalOutbox(t *testing.T) {
	repo, tx, stager := &intakeRepoStub{}, &txStub{}, &stagerStub{}
	service := NewService(repo, domainassessment.NewDefaultAssessmentCreator(), tx, stager, nil)
	created, err := service.CreateForAnswerSheet(context.Background(), CreateCommand{OrgID: 1, TesteeID: 2, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", AnswerSheetID: 3, OriginType: "adhoc"})
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

var _ domainassessment.Repository = (*intakeRepoStub)(nil)
