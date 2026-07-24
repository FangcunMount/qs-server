package answersheet

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type lookupDurableStoreStub struct {
	existing    *domainanswersheet.AnswerSheet
	findErr     error
	findCalls   int
	createCalls int
	lastMeta    DurableSubmitMeta
}

func (s *lookupDurableStoreStub) FindCompleted(_ context.Context, meta DurableSubmitMeta) (*domainanswersheet.AnswerSheet, error) {
	s.findCalls++
	s.lastMeta = meta
	return s.existing, s.findErr
}

func (s *lookupDurableStoreStub) CreateDurably(_ context.Context, sheet *domainanswersheet.AnswerSheet, _ DurableSubmitMeta) (*domainanswersheet.AnswerSheet, bool, error) {
	s.createCalls++
	return sheet, false, nil
}

func TestLookupAcceptedSubmissionReturnsDurableHitWithoutMutableDependencies(t *testing.T) {
	store := &lookupDurableStoreStub{existing: lookupSubmissionTestSheet(t)}
	svc := &submissionService{durableStore: store}

	result, found, err := svc.LookupAcceptedSubmission(t.Context(), lookupSubmissionTestDTO())
	if err != nil || !found || result == nil || result.ID != 999 {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
	if store.findCalls != 1 || store.createCalls != 0 {
		t.Fatalf("store calls find/create = %d/%d, want 1/0", store.findCalls, store.createCalls)
	}
	if store.lastMeta.WriterID != 301 || store.lastMeta.IdempotencyKey != "idem-existing" || store.lastMeta.Fingerprint != "" {
		t.Fatalf("lookup meta = %+v", store.lastMeta)
	}
}

func TestLookupAcceptedSubmissionReturnsMissWithoutStartingTransaction(t *testing.T) {
	store := &lookupDurableStoreStub{}
	svc := &submissionService{durableStore: store}

	result, found, err := svc.LookupAcceptedSubmission(t.Context(), lookupSubmissionTestDTO())
	if err != nil || found || result != nil {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
	if store.findCalls != 1 || store.createCalls != 0 {
		t.Fatalf("store calls find/create = %d/%d, want 1/0", store.findCalls, store.createCalls)
	}
}

func TestLookupAcceptedSubmissionRejectsDifferentFingerprintBeforeMutableDependencies(t *testing.T) {
	store := &lookupDurableStoreStub{existing: lookupSubmissionTestSheet(t)}
	svc := &submissionService{durableStore: store}
	dto := lookupSubmissionTestDTO()
	dto.Answers[0].Value = "different"

	result, found, err := svc.LookupAcceptedSubmission(t.Context(), dto)
	if result != nil || found || errors.ParseCoder(err).Code() != errorCode.ErrConflict {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
	if store.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", store.createCalls)
	}
}

func TestLookupAcceptedSubmissionDoesNotTreatReadErrorAsMiss(t *testing.T) {
	store := &lookupDurableStoreStub{findErr: context.DeadlineExceeded}
	svc := &submissionService{durableStore: store}

	result, found, err := svc.LookupAcceptedSubmission(t.Context(), lookupSubmissionTestDTO())
	if result != nil || found || err == nil {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v, want read error", result, found, err)
	}
	if errors.ParseCoder(err).Code() != errorCode.ErrDatabase {
		t.Fatalf("error code = %d, want ErrDatabase", errors.ParseCoder(err).Code())
	}
	if store.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", store.createCalls)
	}
}

func lookupSubmissionTestDTO() LookupSubmissionDTO {
	return LookupSubmissionDTO{
		QuestionnaireCode: "QNR-1",
		QuestionnaireVer:  "1.0.0",
		IdempotencyKey:    "idem-existing",
		FillerID:          301,
		TesteeID:          401,
		Answers: []AnswerDTO{{
			QuestionCode: "Q1",
			QuestionType: "Text",
			Value:        "ok",
		}},
	}
}

func lookupSubmissionTestSheet(t *testing.T) *domainanswersheet.AnswerSheet {
	t.Helper()
	ref, err := domainanswersheet.NewQuestionnaireRef("QNR-1", "1.0.0", "Questionnaire")
	if err != nil {
		t.Fatal(err)
	}
	attribution, err := domainanswersheet.NewAttributionSnapshot(
		domainanswersheet.OriginRef{Type: domainanswersheet.OriginTypeSelfService},
		"", "", "", "", "", time.Unix(1, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	submissionContext, err := domainanswersheet.NewSubmissionContextWithAttribution(
		actor.NewFillerRef(301, actor.FillerTypeSelf),
		actor.NewTesteeRef(meta.FromUint64(401)),
		meta.FromUint64(501),
		"",
		attribution,
	)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := domainanswersheet.NewAnswer(
		meta.NewCode("Q1"),
		questionnaire.TypeText,
		domainanswersheet.NewStringValue("ok"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := domainanswersheet.Submit(
		meta.FromUint64(999),
		ref,
		submissionContext,
		[]domainanswersheet.Answer{answer},
		time.Unix(2, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}
