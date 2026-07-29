package historicalseed

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestVerifierRoundTripAndGuards(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, location)
	resolve := time.Date(2025, 1, 1, 8, 5, 0, 0, location)
	intake := resolve.Add(2 * time.Minute)
	historical := Context{
		BatchID: "hist-20250101-20260727-v1", ScenarioID: "2025-01-01/1/submit_answer/scale-a",
		OrgID: 1, Version: Version1, Timeline: Timeline{EntryResolvedAt: &resolve, EntryIntakeAt: &intake},
	}
	encoded, err := Encode(historical)
	if err != nil {
		t.Fatal(err)
	}
	requestedAt := now.Format(time.RFC3339Nano)
	body := []byte(`{"answer":"A"}`)
	headers := Headers{EncodedContext: encoded, RequestedAt: requestedAt}
	headers.Signature = Sign("POST", "/api/v1/answersheets?testee_id=9", body, requestedAt, encoded, []byte("secret"))
	verifier := Verifier{
		Enabled: true, Secret: []byte("secret"), AllowedOrgIDs: map[uint64]struct{}{1: {}},
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, location), Latest: time.Date(2026, 7, 27, 0, 0, 0, 0, location),
		Location: location, MaxSkew: 5 * time.Minute, Now: func() time.Time { return now },
	}
	got, present, err := verifier.Verify("POST", "/api/v1/answersheets?testee_id=9", body, headers)
	if err != nil || !present || got.BatchID != historical.BatchID {
		t.Fatalf("verify round trip: context=%+v present=%v err=%v", got, present, err)
	}

	headers.Signature = Sign("POST", "/api/v1/answersheets?testee_id=9", []byte(`{"answer":"B"}`), requestedAt, encoded, []byte("secret"))
	if _, _, err := verifier.Verify("POST", "/api/v1/answersheets?testee_id=9", body, headers); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected body tampering rejection, got %v", err)
	}
	verifier.Enabled = false
	if _, _, err := verifier.Verify("POST", "/api/v1/answersheets?testee_id=9", body, headers); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected disabled rejection, got %v", err)
	}
}

func TestContextRejectsOutOfOrderAndDateBoundary(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	resolve := time.Date(2025, 1, 2, 9, 0, 0, 0, location)
	intake := resolve.Add(-time.Minute)
	historical := Context{BatchID: "batch", ScenarioID: "scenario", OrgID: 1, Version: Version1, Timeline: Timeline{EntryResolvedAt: &resolve, EntryIntakeAt: &intake}}
	if err := historical.Validate(time.Date(2025, 1, 1, 0, 0, 0, 0, location), time.Date(2026, 7, 27, 0, 0, 0, 0, location), location); err == nil {
		t.Fatal("expected timeline order error")
	}

	future := time.Date(2026, 7, 28, 0, 0, 0, 0, location)
	historical.Timeline = Timeline{EntryResolvedAt: &future}
	if err := historical.Validate(time.Date(2025, 1, 1, 0, 0, 0, 0, location), time.Date(2026, 7, 27, 0, 0, 0, 0, location), location); err == nil {
		t.Fatal("expected date boundary error")
	}
}

func TestContextRequiresPlanTaskCompletionAfterAssessmentSubmission(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	filled := time.Date(2025, 1, 2, 9, 0, 0, 0, location)
	created, submitted := filled.Add(time.Second), filled.Add(2*time.Second)
	completed, evaluated := filled.Add(3*time.Second), filled.Add(5*time.Second)
	historical := Context{
		BatchID: "batch", ScenarioID: "task-scenario", OrgID: 1, Version: Version1,
		Timeline: Timeline{
			AnswerSheetFilledAt: &filled, AssessmentCreatedAt: &created, AssessmentSubmittedAt: &submitted,
			TaskCompletedAt: &completed, EvaluatedAt: &evaluated,
		},
	}
	if err := historical.Validate(time.Time{}, time.Time{}, location); err != nil {
		t.Fatalf("valid task completion order rejected: %v", err)
	}

	beforeAssessment := filled.Add(500 * time.Millisecond)
	historical.Timeline.TaskCompletedAt = &beforeAssessment
	if err := historical.Validate(time.Time{}, time.Time{}, location); err == nil {
		t.Fatal("task completion before assessment submission was accepted")
	}
}

func TestVerifierVerifyForwardedEnforcesFeatureAndScope(t *testing.T) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	at := time.Date(2025, 1, 2, 9, 0, 0, 0, location)
	historical := Context{
		BatchID: "batch", ScenarioID: "scenario", OrgID: 7, Version: Version1,
		Timeline: Timeline{AnswerSheetFilledAt: &at},
	}

	if err := (*Verifier)(nil).VerifyForwarded(historical); !errors.Is(err, ErrDisabled) {
		t.Fatalf("nil verifier error = %v, want ErrDisabled", err)
	}
	verifier := &Verifier{
		Enabled: true, AllowedOrgIDs: map[uint64]struct{}{7: {}}, Location: location,
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, location),
		Latest:   time.Date(2026, 7, 27, 0, 0, 0, 0, location),
	}
	if err := verifier.VerifyForwarded(historical); err != nil {
		t.Fatalf("VerifyForwarded() error = %v", err)
	}

	outside := historical
	future := time.Date(2026, 7, 28, 0, 0, 0, 0, location)
	outside.Timeline.AnswerSheetFilledAt = &future
	if err := verifier.VerifyForwarded(outside); err == nil {
		t.Fatal("VerifyForwarded() accepted an out-of-range timeline")
	}

	wrongOrg := historical
	wrongOrg.OrgID = 8
	if err := verifier.VerifyForwarded(wrongOrg); err == nil {
		t.Fatal("VerifyForwarded() accepted an organization outside the allow-list")
	}
}

func TestOccurredAtUsesHistoricalTimeOnlyForMatchingOrg(t *testing.T) {
	systemNow := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	historicalAt := time.Date(2025, 1, 1, 8, 5, 0, 0, time.UTC)
	ordinary, err := OccurredAt(context.Background(), 1, StageEntryResolved, systemNow)
	if err != nil || !ordinary.Equal(systemNow) {
		t.Fatalf("ordinary occurred-at changed: %v %v", ordinary, err)
	}
	ctx := WithContext(context.Background(), Context{OrgID: 1, Timeline: Timeline{EntryResolvedAt: &historicalAt}})
	got, err := OccurredAt(ctx, 1, StageEntryResolved, systemNow)
	if err != nil || !got.Equal(historicalAt) {
		t.Fatalf("historical occurred-at mismatch: %v %v", got, err)
	}
	if _, err := OccurredAt(ctx, 2, StageEntryResolved, systemNow); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("expected org mismatch, got %v", err)
	}
}

func TestOccurredAtReturnsHistoricalTesteeCreatedAt(t *testing.T) {
	historicalAt := time.Date(2025, 1, 1, 8, 42, 0, 0, time.UTC)
	ctx := WithContext(context.Background(), Context{OrgID: 1, Timeline: Timeline{TesteeCreatedAt: &historicalAt}})
	got, err := OccurredAt(ctx, 1, StageTesteeCreated, time.Now())
	if err != nil || !got.Equal(historicalAt) {
		t.Fatalf("historical testee created-at mismatch: %v %v", got, err)
	}
}
