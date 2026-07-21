package answersheet

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	attributionport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetattribution"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type durableStoreCaptureStub struct {
	lastMeta      DurableSubmitMeta
	lastSheet     *domainAnswerSheet.AnswerSheet
	existing      bool
	returnedSheet *domainAnswerSheet.AnswerSheet
}

type preflightDurableStoreStub struct {
	existing    *domainAnswerSheet.AnswerSheet
	createCalls int
}

func (s *preflightDurableStoreStub) FindCompleted(context.Context, DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, error) {
	return s.existing, nil
}
func (s *preflightDurableStoreStub) CreateDurably(_ context.Context, sheet *domainAnswerSheet.AnswerSheet, _ DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	s.createCalls++
	return sheet, false, nil
}

type attributionResolverCaptureStub struct{ calls int }

func (s *attributionResolverCaptureStub) Resolve(context.Context, attributionport.ResolveRequest) (domainAnswerSheet.AttributionSnapshot, error) {
	s.calls++
	return domainAnswerSheet.AttributionSnapshot{}, context.Canceled
}

func (s *durableStoreCaptureStub) CreateDurably(_ context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	s.lastMeta = meta
	s.lastSheet = sheet
	if s.returnedSheet != nil {
		return s.returnedSheet, s.existing, nil
	}
	return sheet, s.existing, nil
}

func TestSubmissionServiceCreateAndSaveAnswerSheetPassesDurableSubmitMeta(t *testing.T) {
	store := &durableStoreCaptureStub{existing: true}
	svc := &submissionService{durableStore: store}

	qnr, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("QNR-1"),
		"Questionnaire",
		domainQuestionnaire.WithVersion(domainQuestionnaire.Version("1.0.0")),
		domainQuestionnaire.WithStatus(domainQuestionnaire.STATUS_PUBLISHED),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	answer, err := domainAnswerSheet.NewAnswer(meta.NewCode("Q1"), domainQuestionnaire.TypeText, domainAnswerSheet.NewStringValue("ok"), 0)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}

	ctx := context.Background()
	result, err := svc.createAndSaveAnswerSheet(ctx, logger.L(ctx), SubmitAnswerSheetDTO{
		IdempotencyKey:    "idem-1",
		FillerID:          301,
		TesteeID:          401,
		OrgID:             501,
		TaskID:            "task-1",
		QuestionnaireCode: "QNR-1",
		QuestionnaireVer:  "1.0.0",
	}, qnr, []domainAnswerSheet.Answer{answer})
	if err != nil {
		t.Fatalf("createAndSaveAnswerSheet() error = %v", err)
	}
	if result == nil {
		t.Fatal("createAndSaveAnswerSheet() returned nil sheet")
	}
	if store.lastMeta.IdempotencyKey != "idem-1" || store.lastMeta.WriterID != 301 || len(store.lastMeta.Fingerprint) != 64 {
		t.Fatalf("unexpected durable meta: %+v", store.lastMeta)
	}
	if store.lastSheet == nil {
		t.Fatal("durable store did not receive sheet")
	}
	submissionContext := store.lastSheet.SubmissionContext()
	if submissionContext.Filler().UserID() != 301 || submissionContext.TesteeID().Uint64() != 401 || submissionContext.OrgID().Uint64() != 501 || submissionContext.TaskID() != "task-1" {
		t.Fatalf("unexpected submission context: %+v", submissionContext)
	}
}

func TestSubmissionServiceCreateAndSaveAnswerSheetReturnsExistingSheet(t *testing.T) {
	existing := domainAnswerSheet.Reconstruct(
		meta.FromUint64(999),
		mustQuestionnaireRefForSubmissionTest(t),
		nil,
		mustAnswersForSubmissionTest(t),
		nowForSubmissionTest(),
		0,
	)
	store := &durableStoreCaptureStub{existing: true, returnedSheet: existing}
	svc := &submissionService{durableStore: store}
	qnr, _ := domainQuestionnaire.NewQuestionnaire(meta.NewCode("QNR-1"), "Questionnaire")
	result, err := svc.createAndSaveAnswerSheet(context.Background(), logger.L(context.Background()), SubmitAnswerSheetDTO{
		FillerID:          301,
		TesteeID:          401,
		OrgID:             501,
		QuestionnaireCode: "QNR-1",
		QuestionnaireVer:  "1.0.0",
	}, qnr, mustAnswersForSubmissionTest(t))
	if err != nil {
		t.Fatalf("createAndSaveAnswerSheet() error = %v", err)
	}
	if result != existing {
		t.Fatalf("expected existing sheet to be returned")
	}
}

func TestSubmissionServiceReturnsIdempotentAnswerBeforeMutableAttributionRevalidation(t *testing.T) {
	existing := domainAnswerSheet.Reconstruct(meta.FromUint64(999), mustQuestionnaireRefForSubmissionTest(t), nil, mustAnswersForSubmissionTest(t), nowForSubmissionTest(), 0)
	store := &preflightDurableStoreStub{existing: existing}
	resolver := &attributionResolverCaptureStub{}
	svc := &submissionService{durableStore: store, attribution: resolver}
	qnr, _ := domainQuestionnaire.NewQuestionnaire(meta.NewCode("QNR-1"), "Questionnaire")
	result, err := svc.createAndSaveAnswerSheet(context.Background(), logger.L(context.Background()), SubmitAnswerSheetDTO{
		IdempotencyKey: "idem-existing", FillerID: 301, TesteeID: 401, OrgID: 501,
		QuestionnaireCode: "QNR-1", QuestionnaireVer: "1.0.0", OriginRef: &OriginRefDTO{Type: "assessment_entry", ID: "9001"},
	}, qnr, mustAnswersForSubmissionTest(t))
	if err != nil || result != existing {
		t.Fatalf("result=%p existing=%p err=%v", result, existing, err)
	}
	if resolver.calls != 0 || store.createCalls != 0 {
		t.Fatalf("mutable source was revalidated or rewritten: resolver=%d create=%d", resolver.calls, store.createCalls)
	}
}

func mustAnswersForSubmissionTest(t *testing.T) []domainAnswerSheet.Answer {
	t.Helper()
	answer, err := domainAnswerSheet.NewAnswer(meta.NewCode("Q1"), domainQuestionnaire.TypeText, domainAnswerSheet.NewStringValue("ok"), 0)
	if err != nil {
		t.Fatalf("NewAnswer() error = %v", err)
	}
	return []domainAnswerSheet.Answer{answer}
}

func nowForSubmissionTest() time.Time {
	return time.Unix(1, 0)
}
