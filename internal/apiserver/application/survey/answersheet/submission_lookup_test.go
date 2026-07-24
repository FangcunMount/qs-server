package answersheet

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	domainanswersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type lookupDurableStoreStub struct {
	completed   *CompletedSubmission
	findErr     error
	findCalls   int
	createCalls int
	lastMeta    DurableSubmitMeta
}

func (s *lookupDurableStoreStub) FindCompleted(_ context.Context, meta DurableSubmitMeta) (*CompletedSubmission, error) {
	s.findCalls++
	s.lastMeta = meta
	return s.completed, s.findErr
}

func (s *lookupDurableStoreStub) CreateDurably(_ context.Context, sheet *domainanswersheet.AnswerSheet, _ DurableSubmitMeta) (*domainanswersheet.AnswerSheet, bool, error) {
	s.createCalls++
	return sheet, false, nil
}

func TestLookupAcceptedSubmissionReturnsDurableHitWithoutMutableDependencies(t *testing.T) {
	sheet := lookupSubmissionTestSheet(t)
	store := &lookupDurableStoreStub{completed: completedSubmissionForTest(t, sheet)}
	svc := &submissionService{durableStore: store}

	beforeExplicitHit := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "hit"))
	beforeEarlyHit := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("early_lookup", "hit"))
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
	if delta := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "hit")) - beforeExplicitHit; delta != 1 {
		t.Fatalf("explicit_readback hit metric delta = %v, want 1", delta)
	}
	if delta := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("early_lookup", "hit")) - beforeEarlyHit; delta != 0 {
		t.Fatalf("explicit lookup changed early_lookup hit by %v, want 0", delta)
	}
}

func TestLookupAcceptedSubmissionReturnsMissWithoutStartingTransaction(t *testing.T) {
	store := &lookupDurableStoreStub{}
	svc := &submissionService{durableStore: store}

	before := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "miss"))
	result, found, err := svc.LookupAcceptedSubmission(t.Context(), lookupSubmissionTestDTO())
	if err != nil || found || result != nil {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
	if store.findCalls != 1 || store.createCalls != 0 {
		t.Fatalf("store calls find/create = %d/%d, want 1/0", store.findCalls, store.createCalls)
	}
	if delta := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "miss")) - before; delta != 1 {
		t.Fatalf("explicit_readback miss metric delta = %v, want 1", delta)
	}
}

func TestLookupAcceptedSubmissionRejectsDifferentFingerprintBeforeMutableDependencies(t *testing.T) {
	sheet := lookupSubmissionTestSheet(t)
	store := &lookupDurableStoreStub{completed: completedSubmissionForTest(t, sheet)}
	svc := &submissionService{durableStore: store}
	dto := lookupSubmissionTestDTO()
	dto.Answers[0].Value = "different"

	before := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "conflict"))
	result, found, err := svc.LookupAcceptedSubmission(t.Context(), dto)
	if result != nil || found || errors.ParseCoder(err).Code() != errorCode.ErrConflict {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
	if store.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", store.createCalls)
	}
	if delta := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "conflict")) - before; delta != 1 {
		t.Fatalf("explicit_readback conflict metric delta = %v, want 1", delta)
	}
}

func TestLookupAcceptedSubmissionUsesPersistedFingerprintAsHistoricalFact(t *testing.T) {
	sheet := lookupSubmissionTestSheet(t)
	dto := lookupSubmissionTestDTO()
	dto.Answers[0].Value = "historically-accepted"
	persistedFingerprint, err := lookupSubmissionFingerprint(sheet, dto)
	if err != nil {
		t.Fatal(err)
	}
	reconstructedFingerprint, err := submitport.Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if persistedFingerprint == reconstructedFingerprint {
		t.Fatal("test requires persisted fingerprint to differ from current AnswerSheet projection")
	}

	store := &lookupDurableStoreStub{completed: &CompletedSubmission{
		Sheet:       sheet,
		Fingerprint: persistedFingerprint,
	}}
	svc := &submissionService{durableStore: store}

	result, found, err := svc.LookupAcceptedSubmission(t.Context(), dto)
	if err != nil || !found || result == nil || result.ID != 999 {
		t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v", result, found, err)
	}
}

func TestLookupAcceptedSubmissionRejectsIncompletePersistedRecord(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		completed *CompletedSubmission
	}{
		{name: "missing sheet", completed: &CompletedSubmission{Fingerprint: "persisted"}},
		{name: "missing fingerprint", completed: &CompletedSubmission{Sheet: lookupSubmissionTestSheet(t)}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := &lookupDurableStoreStub{completed: testCase.completed}
			svc := &submissionService{durableStore: store}

			result, found, err := svc.LookupAcceptedSubmission(t.Context(), lookupSubmissionTestDTO())
			if result != nil || found || errors.ParseCoder(err).Code() != errorCode.ErrDatabase {
				t.Fatalf("LookupAcceptedSubmission() = result=%#v found=%v err=%v, want database error", result, found, err)
			}
		})
	}
}

func TestLookupAcceptedSubmissionDoesNotTreatReadErrorAsMiss(t *testing.T) {
	store := &lookupDurableStoreStub{findErr: context.DeadlineExceeded}
	svc := &submissionService{durableStore: store}

	before := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "error"))
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
	if delta := testutil.ToFloat64(durableSubmitOperationTotal.WithLabelValues("explicit_readback", "error")) - before; delta != 1 {
		t.Fatalf("explicit_readback error metric delta = %v, want 1", delta)
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

func completedSubmissionForTest(t *testing.T, sheet *domainanswersheet.AnswerSheet) *CompletedSubmission {
	t.Helper()
	fingerprint, err := submitport.Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return &CompletedSubmission{Sheet: sheet, Fingerprint: fingerprint}
}
