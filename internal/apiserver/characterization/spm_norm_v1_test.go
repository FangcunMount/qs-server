package characterization_test

import (
	"context"
	"strings"
	"testing"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestV1SPMExecuteCommitsStandardScoreAndActualNormReference(t *testing.T) {
	t.Parallel()

	a := draftSPMAssessment(t)
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()
	svc, _ := newV1RecordingExecuteService(t, a, &charInputResolver{snapshot: spmNormInputSnapshot()})
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	result := svc.capture.outcome.Execution
	if result == nil || len(result.Dimensions) == 0 {
		t.Fatalf("committed outcome = %#v", result)
	}
	total := result.Dimensions[len(result.Dimensions)-1]
	if got := charDerivedScore(total.DerivedScores, evaluationfact.ScoreKindStandardScore); got != 110 {
		t.Fatalf("standard score = %v, want 110", got)
	}
	if total.NormReference == nil || total.NormReference.TableVersion != "spm-cn-2026" || total.NormReference.FormVariant != "standard" || total.NormReference.MinAgeMonths != 60 || total.NormReference.MaxAgeMonths != 95 || total.NormReference.Gender != "female" {
		t.Fatalf("norm reference = %#v", total.NormReference)
	}
}

func TestV1RequiredNormFailureHasNoOutcomeThenForceRetryCreatesAuditedV2Revision(t *testing.T) {
	t.Parallel()

	a := draftSPMAssessment(t)
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()
	missingSubject := spmNormInputSnapshot()
	missingSubject.NormSubject = nil
	resolver := &charInputResolver{snapshot: missingSubject}
	svc, _ := newV1RecordingExecuteService(t, a, resolver)

	firstErr := svc.Evaluate(context.Background(), a.ID().Uint64())
	if firstErr == nil {
		t.Fatal("first Evaluate error = nil, want norm_subject_missing")
	}
	if svc.capture.outcome.Execution != nil {
		t.Fatalf("outcome committed on failed attempt: %#v", svc.capture.outcome.Execution)
	}
	if svc.runRepo.latest == nil || svc.runRepo.latest.Failure() == nil || svc.runRepo.latest.Failure().Kind != evalrun.FailureKindNormSubjectMissing || svc.runRepo.latest.Failure().Retryable {
		t.Fatalf("first run = %#v", svc.runRepo.latest)
	}
	previousRef := svc.runRepo.latest.InputSnapshotRef()
	if previousRef == "" {
		t.Fatal("first attempt input snapshot ref is empty")
	}

	resolver.snapshot = spmNormInputSnapshot()
	forceCtx := retrygovernance.WithAuthorization(context.Background(), retrygovernance.Authorization{
		EventID: "force-event-1", ExpectedAttempt: 1, Origin: retrygovernance.AttemptOriginForce,
		ActionRequestID: "force-request-1", Mode: "next_attempt",
	})
	if err := svc.Evaluate(forceCtx, a.ID().Uint64()); err != nil {
		t.Fatalf("force Evaluate: %v", err)
	}
	if svc.runRepo.latest == nil || svc.runRepo.latest.Origin() != retrygovernance.AttemptOriginForce || svc.runRepo.latest.ActionRequestID() != "force-request-1" {
		t.Fatalf("force run = %#v", svc.runRepo.latest)
	}
	if currentRef := svc.runRepo.latest.InputSnapshotRef(); currentRef == previousRef || !strings.HasPrefix(currentRef, "isn:v2:") {
		t.Fatalf("force input ref = %q previous=%q", currentRef, previousRef)
	}
	result := svc.capture.outcome.Execution
	if result == nil || result.Dimensions[len(result.Dimensions)-1].NormReference == nil {
		t.Fatalf("force outcome = %#v", result)
	}
}
