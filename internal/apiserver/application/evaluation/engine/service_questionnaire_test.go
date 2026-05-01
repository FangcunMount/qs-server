package engine

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type fakeAssessmentRepo struct {
	assessment         *domainAssessment.Assessment
	saveCalls          int
	eventfulSaveCalls  int
	saveCtxHadTxMarker bool
}

type engineTxCtxMarker struct{}

type engineRecordingTxRunner struct {
	called bool
}

func (r *engineRecordingTxRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	return fn(context.WithValue(ctx, engineTxCtxMarker{}, true))
}

type engineRecordingEventStager struct {
	ctxHadTxMarker bool
	eventTypes     []string
}

func (s *engineRecordingEventStager) Stage(ctx context.Context, events ...event.DomainEvent) error {
	s.ctxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return nil
}

func (r *fakeAssessmentRepo) Save(ctx context.Context, assessment *domainAssessment.Assessment) error {
	r.saveCtxHadTxMarker, _ = ctx.Value(engineTxCtxMarker{}).(bool)
	r.assessment = assessment
	r.saveCalls++
	return nil
}

func (r *fakeAssessmentRepo) SaveWithEvents(_ context.Context, assessment *domainAssessment.Assessment) error {
	r.assessment = assessment
	r.eventfulSaveCalls++
	r.saveCalls++
	assessment.ClearEvents()
	return nil
}

func (r *fakeAssessmentRepo) SaveWithAdditionalEvents(_ context.Context, assessment *domainAssessment.Assessment, _ []event.DomainEvent) error {
	r.assessment = assessment
	r.eventfulSaveCalls++
	r.saveCalls++
	assessment.ClearEvents()
	return nil
}

func (r *fakeAssessmentRepo) FindByID(_ context.Context, _ domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return r.assessment, nil
}

func (r *fakeAssessmentRepo) Delete(_ context.Context, _ domainAssessment.ID) error { return nil }
func (r *fakeAssessmentRepo) FindByAnswerSheetID(_ context.Context, _ domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *fakeAssessmentRepo) FindByTesteeID(_ context.Context, _ testee.ID, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByTesteeIDWithFilters(_ context.Context, _ testee.ID, _ string, _ string, _ string, _ *time.Time, _ *time.Time, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByTesteeIDAndScaleID(_ context.Context, _ testee.ID, _ domainAssessment.MedicalScaleRef, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByPlanID(_ context.Context, _ string, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) CountByStatus(_ context.Context, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) CountByTesteeIDAndStatus(_ context.Context, _ testee.ID, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) CountByOrgIDAndStatus(_ context.Context, _ int64, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) FindByIDs(_ context.Context, _ []domainAssessment.ID) ([]*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *fakeAssessmentRepo) FindPendingSubmission(_ context.Context, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByOrgID(_ context.Context, _ int64, _ *domainAssessment.Status, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByOrgIDAndTesteeIDs(_ context.Context, _ int64, _ []testee.ID, _ *domainAssessment.Status, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}

type fakeScaleRepo struct {
	scale *domainScale.MedicalScale
}

func (r *fakeScaleRepo) Create(_ context.Context, _ *domainScale.MedicalScale) error { return nil }
func (r *fakeScaleRepo) FindByCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return r.scale, nil
}
func (r *fakeScaleRepo) FindByQuestionnaireCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return nil, nil
}
func (r *fakeScaleRepo) Update(_ context.Context, _ *domainScale.MedicalScale) error { return nil }
func (r *fakeScaleRepo) Remove(_ context.Context, _ string) error                    { return nil }
func (r *fakeScaleRepo) ExistsByCode(_ context.Context, _ string) (bool, error)      { return false, nil }

type fakeAnswerSheetRepo struct {
	answerSheet *domainAnswerSheet.AnswerSheet
}

func (r *fakeAnswerSheetRepo) Create(_ context.Context, _ *domainAnswerSheet.AnswerSheet) error {
	return nil
}
func (r *fakeAnswerSheetRepo) Update(_ context.Context, _ *domainAnswerSheet.AnswerSheet) error {
	return nil
}
func (r *fakeAnswerSheetRepo) FindByID(_ context.Context, _ meta.ID) (*domainAnswerSheet.AnswerSheet, error) {
	return r.answerSheet, nil
}
func (r *fakeAnswerSheetRepo) Delete(_ context.Context, _ meta.ID) error { return nil }

type fakeQuestionnaireRepo struct{}

func (r *fakeQuestionnaireRepo) Create(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) FindByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindLatestPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBaseByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBasePublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBaseByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) LoadQuestions(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) Update(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) CreatePublishedSnapshot(_ context.Context, _ *domainQuestionnaire.Questionnaire, _ bool) error {
	return nil
}
func (r *fakeQuestionnaireRepo) SetActivePublishedVersion(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) ClearActivePublishedVersion(_ context.Context, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) Remove(_ context.Context, _ string) error     { return nil }
func (r *fakeQuestionnaireRepo) HardDelete(_ context.Context, _ string) error { return nil }
func (r *fakeQuestionnaireRepo) HardDeleteFamily(_ context.Context, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *fakeQuestionnaireRepo) HasPublishedSnapshots(_ context.Context, _ string) (bool, error) {
	return false, nil
}

var _ domainAssessment.ScoreRepository = (*noopScoreRepo)(nil)
var _ domainReport.ReportRepository = (*noopReportRepo)(nil)

type noopScoreRepo struct{}

func (r *noopScoreRepo) SaveScores(_ context.Context, _ []*domainAssessment.AssessmentScore) error {
	return nil
}
func (r *noopScoreRepo) SaveScoresWithContext(_ context.Context, _ *domainAssessment.Assessment, _ *domainAssessment.AssessmentScore) error {
	return nil
}
func (r *noopScoreRepo) FindByAssessmentID(_ context.Context, _ domainAssessment.ID) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) FindByTesteeIDAndFactorCode(_ context.Context, _ testee.ID, _ domainAssessment.FactorCode, _ int) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) FindLatestByTesteeIDAndScaleID(_ context.Context, _ testee.ID, _ domainAssessment.MedicalScaleRef) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) DeleteByAssessmentID(_ context.Context, _ domainAssessment.ID) error {
	return nil
}

type noopReportRepo struct{}

func (r *noopReportRepo) Save(_ context.Context, _ *domainReport.InterpretReport) error { return nil }
func (r *noopReportRepo) SaveWithTesteeAndEvents(_ context.Context, _ *domainReport.InterpretReport, _ testee.ID, _ []event.DomainEvent) error {
	return nil
}
func (r *noopReportRepo) FindByID(_ context.Context, _ domainReport.ID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *noopReportRepo) FindByAssessmentID(_ context.Context, _ domainReport.AssessmentID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *noopReportRepo) FindByTesteeID(_ context.Context, _ testee.ID, _ domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *noopReportRepo) FindByTesteeIDs(_ context.Context, _ []testee.ID, _ domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *noopReportRepo) Update(_ context.Context, _ *domainReport.InterpretReport) error { return nil }
func (r *noopReportRepo) Delete(_ context.Context, _ domainReport.ID) error               { return nil }
func (r *noopReportRepo) ExistsByID(_ context.Context, _ domainReport.ID) (bool, error) {
	return false, nil
}

func TestEvaluateFailsWhenQuestionnaireVersionDoesNotResolveCurrentQuestionnaire(t *testing.T) {
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			ptr(domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	scaleDomain, err := domainScale.NewMedicalScale(
		meta.NewCode("S-001"),
		"Scale",
		domainScale.WithQuestionnaire(meta.NewCode("Q-001"), "0.9.0"),
		domainScale.WithStatus(domainScale.StatusPublished),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}

	answerSheet := domainAnswerSheet.Reconstruct(
		meta.FromUint64(303),
		domainAnswerSheet.NewQuestionnaireRef("Q-001", "0.9.0", "Questionnaire"),
		nil,
		nil,
		time.Now(),
		0,
	)

	svc := &service{
		assessmentRepo: aRepo,
		scoreRepo:      &noopScoreRepo{},
		reportRepo:     &noopReportRepo{},
		inputResolver: NewRepositoryInputResolver(
			&fakeScaleRepo{scale: scaleDomain},
			&fakeAnswerSheetRepo{answerSheet: answerSheet},
			&fakeQuestionnaireRepo{},
		),
		txRunner:    &engineRecordingTxRunner{},
		eventStager: &engineRecordingEventStager{},
	}

	err = svc.Evaluate(context.Background(), 101)
	if err == nil {
		t.Fatal("Evaluate() error = nil, want questionnaire version failure")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls == 0 {
		t.Fatal("assessment should be persisted after markAsFailed")
	}
	if aRepo.eventfulSaveCalls != 0 {
		t.Fatal("deprecated SaveWithEvents fallback should not be used")
	}
	if !aRepo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
}

func TestSaveAssessmentWithEventsRequiresTransactionalOutbox(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}
	repo := &fakeAssessmentRepo{}
	svc := &service{assessmentRepo: repo}

	err := svc.saveAssessmentWithEvents(context.Background(), a)
	if err == nil {
		t.Fatal("expected missing transactional outbox dependencies to fail")
	}
	if repo.saveCalls != 0 {
		t.Fatalf("repository save calls = %d, want 0", repo.saveCalls)
	}
	if repo.eventfulSaveCalls != 0 {
		t.Fatal("deprecated SaveWithEvents fallback should not be used")
	}
}

func TestSaveAssessmentWithEventsStagesThroughApplicationTransaction(t *testing.T) {
	a := engineAssessmentForOutboxTest(t)
	if err := a.MarkAsFailed("pipeline failed"); err != nil {
		t.Fatalf("MarkAsFailed returned error: %v", err)
	}
	repo := &fakeAssessmentRepo{}
	txRunner := &engineRecordingTxRunner{}
	stager := &engineRecordingEventStager{}
	svc := &service{assessmentRepo: repo, txRunner: txRunner, eventStager: stager}

	if err := svc.saveAssessmentWithEvents(context.Background(), a); err != nil {
		t.Fatalf("saveAssessmentWithEvents returned error: %v", err)
	}
	if !txRunner.called {
		t.Fatal("expected transaction runner to be used")
	}
	if repo.saveCalls != 1 {
		t.Fatalf("repository save calls = %d, want 1", repo.saveCalls)
	}
	if repo.eventfulSaveCalls != 0 {
		t.Fatal("deprecated SaveWithEvents fallback should not be used")
	}
	if !repo.saveCtxHadTxMarker {
		t.Fatal("assessment Save should receive transaction context")
	}
	if !stager.ctxHadTxMarker {
		t.Fatal("event stager should receive transaction context")
	}
	if len(stager.eventTypes) != 1 || stager.eventTypes[0] != domainAssessment.EventTypeFailed {
		t.Fatalf("staged event types = %#v, want assessment failed", stager.eventTypes)
	}
	if len(a.Events()) != 0 {
		t.Fatalf("expected events to be cleared after successful transaction, got %d", len(a.Events()))
	}
}

func engineAssessmentForOutboxTest(t *testing.T) *domainAssessment.Assessment {
	t.Helper()
	return domainAssessment.Reconstruct(
		meta.FromUint64(9901),
		1,
		testee.NewID(202),
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
		ptr(domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.StatusSubmitted,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func ptr[T any](v T) *T {
	return &v
}

type waiterNotifierStub struct{}

func (w *waiterNotifierStub) Notify(context.Context, uint64, evaluationwaiter.StatusSummary) {}

func (w *waiterNotifierStub) GetWaiterCount(uint64) int { return 0 }

type noopReportBuilder struct{}

func (b *noopReportBuilder) Build(*domainAssessment.Assessment, *domainScale.MedicalScale, *domainAssessment.EvaluationResult) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func TestNewServiceAcceptsWaiterPort(t *testing.T) {
	waiterRegistry := &waiterNotifierStub{}

	svc := NewService(
		&fakeAssessmentRepo{},
		&noopScoreRepo{},
		&noopReportRepo{},
		&fakeScaleRepo{},
		&fakeAnswerSheetRepo{},
		&fakeQuestionnaireRepo{},
		&noopReportBuilder{},
		WithWaiterRegistry(waiterRegistry),
	)

	impl, ok := svc.(*service)
	if !ok {
		t.Fatalf("expected *service, got %T", svc)
	}
	if impl.waiterRegistry != waiterRegistry {
		t.Fatal("expected waiter registry port to be stored on service")
	}
	if impl.pipeline == nil {
		t.Fatal("expected pipeline to be initialized")
	}
}
