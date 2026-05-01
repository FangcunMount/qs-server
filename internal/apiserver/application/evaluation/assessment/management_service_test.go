package assessment

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type managementRepoStub struct {
	assessment      *domain.Assessment
	saved           *domain.Assessment
	savedWithEvents *domain.Assessment
	savedEventTypes []string

	findByTesteeIDAssessments []*domain.Assessment
	findByTesteeIDTotal       int64

	findByOrgIDAssessments []*domain.Assessment
	findByOrgIDTotal       int64

	findByScopeAssessments []*domain.Assessment
	findByScopeTotal       int64
	findByScopeOrgID       int64
	findByScopeTesteeIDs   []testee.ID
	findByScopeStatus      *domain.Status
	saveCtxHadTxMarker     bool
}

type managementAssessmentReaderStub struct {
	rows   []evaluationreadmodel.AssessmentRow
	total  int64
	filter evaluationreadmodel.AssessmentFilter
	page   evaluationreadmodel.PageRequest
}

func (r *managementAssessmentReaderStub) GetAssessment(context.Context, uint64) (*evaluationreadmodel.AssessmentRow, error) {
	return nil, fmt.Errorf("assessment not found")
}

func (r *managementAssessmentReaderStub) GetAssessmentByAnswerSheetID(context.Context, uint64) (*evaluationreadmodel.AssessmentRow, error) {
	return nil, fmt.Errorf("assessment not found")
}

func (r *managementAssessmentReaderStub) ListAssessments(_ context.Context, filter evaluationreadmodel.AssessmentFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.AssessmentRow, int64, error) {
	r.filter = filter
	r.page = page
	return r.rows, r.total, nil
}

type txCtxMarker struct{}

type recordingTxRunner struct {
	called bool
}

func (r *recordingTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	return fn(context.WithValue(ctx, txCtxMarker{}, true))
}

type recordingEventStager struct {
	ctxHadTxMarker bool
	eventTypes     []string
}

func (s *recordingEventStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.ctxHadTxMarker, _ = ctx.Value(txCtxMarker{}).(bool)
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return nil
}

func (r *managementRepoStub) Save(ctx context.Context, a *domain.Assessment) error {
	r.saveCtxHadTxMarker, _ = ctx.Value(txCtxMarker{}).(bool)
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
	return r.findByTesteeIDAssessments, r.findByTesteeIDTotal, nil
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
	return r.findByOrgIDAssessments, r.findByOrgIDTotal, nil
}
func (r *managementRepoStub) FindByOrgIDAndTesteeIDs(_ context.Context, orgID int64, testeeIDs []testee.ID, status *domain.Status, _ domain.Pagination) ([]*domain.Assessment, int64, error) {
	r.findByScopeOrgID = orgID
	r.findByScopeTesteeIDs = append([]testee.ID(nil), testeeIDs...)
	r.findByScopeStatus = status
	return r.findByScopeAssessments, r.findByScopeTotal, nil
}

func TestManagementServiceRetryRequiresTransactionalOutbox(t *testing.T) {
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

	if _, err := svc.Retry(context.Background(), 9, id.Uint64()); err == nil {
		t.Fatal("expected Retry to fail when transactional outbox is not configured")
	}
	if repo.saved != nil {
		t.Fatal("expected repository Save not to be called without transactional outbox")
	}
	if repo.savedWithEvents != nil {
		t.Fatal("expected deprecated SaveWithEvents fallback not to be called")
	}
}

func TestSaveAssessmentAndStageEventsRequiresCompleteTransactionalOutboxConfig(t *testing.T) {
	submittedAt := time.Now().Add(-time.Hour)
	a := domain.Reconstruct(
		domain.NewID(9002),
		9,
		testee.NewID(3002),
		domain.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domain.NewAnswerSheetRef(meta.FromUint64(4002)),
		nil,
		domain.NewAdhocOrigin(),
		domain.StatusSubmitted,
		nil,
		nil,
		&submittedAt,
		nil,
		nil,
		nil,
	)
	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}

	assertMissingDependency := func(t *testing.T, repo *managementRepoStub, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected missing transactional outbox dependency to fail")
		}
		if repo.saved != nil {
			t.Fatal("expected repository Save not to be called")
		}
		if repo.savedWithEvents != nil {
			t.Fatal("expected deprecated eventful save fallback not to be called")
		}
	}

	t.Run("missing tx runner", func(t *testing.T) {
		repo := &managementRepoStub{}
		err := saveAssessmentAndStageEvents(context.Background(), repo, nil, &recordingEventStager{}, a, nil)
		assertMissingDependency(t, repo, err)
	})
	t.Run("missing stager", func(t *testing.T) {
		repo := &managementRepoStub{}
		err := saveAssessmentAndStageEvents(context.Background(), repo, &recordingTxRunner{}, nil, a, nil)
		assertMissingDependency(t, repo, err)
	})
}

func TestManagementServiceRetryStagesEventsThroughApplicationTransaction(t *testing.T) {
	id := domain.NewID(9101)
	submittedAt := time.Now().Add(-time.Hour)
	failedAt := time.Now().Add(-time.Minute)
	reason := "pipeline failed"
	a := domain.Reconstruct(
		id,
		9,
		testee.NewID(3001),
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
	txRunner := &recordingTxRunner{}
	stager := &recordingEventStager{}
	svc := NewManagementServiceWithTransactionalOutbox(repo, nil, txRunner, stager)

	if _, err := svc.Retry(context.Background(), 9, id.Uint64()); err != nil {
		t.Fatalf("Retry returned error: %v", err)
	}
	if !txRunner.called {
		t.Fatal("expected application transaction runner to be used")
	}
	if !repo.saveCtxHadTxMarker {
		t.Fatal("expected repository Save to receive transaction context")
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("expected outbox stager to receive transaction context")
	}
	if repo.savedWithEvents != nil {
		t.Fatal("expected compatibility SaveWithEvents path not to be used")
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domain.EventTypeSubmitted {
		t.Fatalf("staged event types = %#v, want assessment submitted", stager.eventTypes)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("expected events to be cleared after successful transaction, got %d", len(a.Events()))
	}
}

func TestManagementServiceListFiltersTesteeAssessmentsByOrgAndStatus(t *testing.T) {
	submitted := domain.StatusSubmitted
	reader := &managementAssessmentReaderStub{
		rows: []evaluationreadmodel.AssessmentRow{
			managementAssessmentRowForList(9001, 9, 3001, domain.StatusSubmitted),
		},
		total: 3,
	}
	svc := NewManagementServiceWithReadModel(&managementRepoStub{}, reader, nil)

	result, err := svc.List(context.Background(), ListAssessmentsDTO{
		OrgID:    9,
		Page:     0,
		PageSize: 0,
		Conditions: map[string]string{
			"testee_id": "3001",
			"status":    submitted.String(),
		},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 filtered assessment, got %d", len(result.Items))
	}
	if result.Items[0].ID != 9001 {
		t.Fatalf("expected assessment 9001, got %d", result.Items[0].ID)
	}
	if result.Page != 1 || result.PageSize != 10 {
		t.Fatalf("expected normalized pagination 1/10, got %d/%d", result.Page, result.PageSize)
	}
	if result.Total != 3 {
		t.Fatalf("expected total to keep read model count 3, got %d", result.Total)
	}
	if reader.filter.TesteeID == nil || *reader.filter.TesteeID != 3001 {
		t.Fatalf("expected read model testee filter 3001, got %+v", reader.filter)
	}
	if len(reader.filter.Statuses) != 1 || reader.filter.Statuses[0] != submitted.String() {
		t.Fatalf("expected submitted status filter, got %+v", reader.filter.Statuses)
	}
}

func TestManagementServiceListUsesAccessScopeStatusFilter(t *testing.T) {
	submitted := domain.StatusSubmitted
	reader := &managementAssessmentReaderStub{
		rows: []evaluationreadmodel.AssessmentRow{
			managementAssessmentRowForList(9001, 9, 3001, submitted),
		},
		total: 1,
	}
	svc := NewManagementServiceWithReadModel(&managementRepoStub{}, reader, nil)

	_, err := svc.List(context.Background(), ListAssessmentsDTO{
		OrgID:                 9,
		Page:                  1,
		PageSize:              20,
		AccessibleTesteeIDs:   []uint64{3001, 3002},
		RestrictToAccessScope: true,
		Conditions: map[string]string{
			"status": submitted.String(),
		},
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if reader.filter.OrgID != 9 {
		t.Fatalf("expected scope org id 9, got %d", reader.filter.OrgID)
	}
	if len(reader.filter.AccessibleTesteeIDs) != 2 {
		t.Fatalf("expected 2 scoped testee IDs, got %d", len(reader.filter.AccessibleTesteeIDs))
	}
	if len(reader.filter.Statuses) != 1 || reader.filter.Statuses[0] != submitted.String() {
		t.Fatalf("expected submitted status filter, got %#v", reader.filter.Statuses)
	}
}

func managementAssessmentRowForList(id uint64, orgID int64, testeeID uint64, status domain.Status) evaluationreadmodel.AssessmentRow {
	return evaluationreadmodel.AssessmentRow{
		ID:                   id,
		OrgID:                orgID,
		TesteeID:             testeeID,
		QuestionnaireCode:    "q-code",
		QuestionnaireVersion: "v1",
		AnswerSheetID:        id + 1000,
		OriginType:           string(domain.OriginAdhoc),
		Status:               status.String(),
	}
}

func managementAssessmentForList(id uint64, orgID int64, testeeID uint64, status domain.Status) *domain.Assessment {
	return domain.Reconstruct(
		domain.NewID(id),
		orgID,
		testee.NewID(testeeID),
		domain.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domain.NewAnswerSheetRef(meta.FromUint64(id+1000)),
		nil,
		domain.NewAdhocOrigin(),
		status,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}
