package answersheet

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type durableStoreCaptureStub struct {
	lastMeta      DurableSubmitMeta
	existing      bool
	returnedSheet *domainAnswerSheet.AnswerSheet
}

func (s *durableStoreCaptureStub) CreateDurably(_ context.Context, sheet *domainAnswerSheet.AnswerSheet, meta DurableSubmitMeta) (*domainAnswerSheet.AnswerSheet, bool, error) {
	s.lastMeta = meta
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
	if store.lastMeta.IdempotencyKey != "idem-1" || store.lastMeta.WriterID != 301 || store.lastMeta.TesteeID != 401 || store.lastMeta.OrgID != 501 || store.lastMeta.TaskID != "task-1" {
		t.Fatalf("unexpected durable meta: %+v", store.lastMeta)
	}
}

func TestSubmissionServiceCreateAndSaveAnswerSheetReturnsExistingSheet(t *testing.T) {
	existing := domainAnswerSheet.Reconstruct(
		meta.FromUint64(999),
		domainAnswerSheet.NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire"),
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
