package assessment

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type managementRepoStub struct {
	assessment      *domain.Assessment
	saved           *domain.Assessment
	savedWithEvents *domain.Assessment
	savedEventTypes []string
}

func (r *managementRepoStub) Save(_ context.Context, a *domain.Assessment) error {
	r.saved = a
	r.assessment = a
	return nil
}

func (r *managementRepoStub) SaveWithEvents(_ context.Context, a *domain.Assessment) error {
	r.savedWithEvents = a
	r.assessment = a
	r.savedEventTypes = r.savedEventTypes[:0]
	for _, evt := range a.Events() {
		r.savedEventTypes = append(r.savedEventTypes, evt.EventType())
	}
	a.ClearEvents()
	return nil
}

func (r *managementRepoStub) SaveWithAdditionalEvents(_ context.Context, a *domain.Assessment, additional []event.DomainEvent) error {
	r.savedWithEvents = a
	r.assessment = a
	r.savedEventTypes = r.savedEventTypes[:0]
	for _, evt := range a.Events() {
		r.savedEventTypes = append(r.savedEventTypes, evt.EventType())
	}
	for _, evt := range additional {
		r.savedEventTypes = append(r.savedEventTypes, evt.EventType())
	}
	a.ClearEvents()
	return nil
}

func (r *managementRepoStub) FindByID(_ context.Context, id domain.ID) (*domain.Assessment, error) {
	if r.assessment != nil && r.assessment.ID() == id {
		return r.assessment, nil
	}
	return nil, fmt.Errorf("assessment not found")
}

func (r *managementRepoStub) Delete(context.Context, domain.ID) error { return nil }
func (r *managementRepoStub) FindByAnswerSheetID(context.Context, domain.AnswerSheetRef) (*domain.Assessment, error) {
	return nil, fmt.Errorf("assessment not found")
}
func (r *managementRepoStub) FindByTesteeID(context.Context, testee.ID, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) FindByTesteeIDWithFilters(context.Context, testee.ID, string, string, string, *time.Time, *time.Time, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) FindByTesteeIDAndScaleID(context.Context, testee.ID, domain.MedicalScaleRef, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) FindByPlanID(context.Context, string, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) CountByStatus(context.Context, domain.Status) (int64, error) {
	return 0, nil
}
func (r *managementRepoStub) CountByTesteeIDAndStatus(context.Context, testee.ID, domain.Status) (int64, error) {
	return 0, nil
}
func (r *managementRepoStub) CountByOrgIDAndStatus(context.Context, int64, domain.Status) (int64, error) {
	return 0, nil
}
func (r *managementRepoStub) FindByIDs(context.Context, []domain.ID) ([]*domain.Assessment, error) {
	return nil, nil
}
func (r *managementRepoStub) FindPendingSubmission(context.Context, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) FindByOrgID(context.Context, int64, *domain.Status, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *managementRepoStub) FindByOrgIDAndTesteeIDs(context.Context, int64, []testee.ID, *domain.Status, domain.Pagination) ([]*domain.Assessment, int64, error) {
	return nil, 0, nil
}

func TestManagementServiceRetryPublishesAssessmentSubmitted(t *testing.T) {
	id := domain.NewID(9001)
	testeeID := testee.NewID(3001)
	submittedAt := time.Now().Add(-time.Hour)
	failedAt := time.Now().Add(-time.Minute)
	reason := "pipeline failed"

	a := domain.Reconstruct(
		id,
		9,
		testeeID,
		domain.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domain.NewAnswerSheetRef(meta.FromUint64(4001)),
		nil,
		domain.NewAdhocOrigin(),
		domain.StatusFailed,
		nil,
		nil,
		&submittedAt,
		nil,
		&failedAt,
		&reason,
	)

	repo := &managementRepoStub{assessment: a}
	svc := NewManagementService(repo, nil)

	result, err := svc.Retry(context.Background(), 9, id.Uint64())
	if err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}

	if result.Status != domain.StatusSubmitted.String() {
		t.Fatalf("expected submitted result status, got %s", result.Status)
	}
	if repo.savedWithEvents == nil {
		t.Fatalf("expected retried assessment to be saved with outbox events")
	}
	if !repo.savedWithEvents.Status().IsSubmitted() {
		t.Fatalf("expected saved assessment to be submitted, got %s", repo.savedWithEvents.Status())
	}
	if len(repo.savedEventTypes) != 1 {
		t.Fatalf("expected one staged event, got %d", len(repo.savedEventTypes))
	}
	if repo.savedEventTypes[0] != domain.EventTypeSubmitted {
		t.Fatalf("expected assessment.submitted event, got %s", repo.savedEventTypes[0])
	}
}
