package answersheet

import (
	"errors"
	"testing"

	submitport "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
)

func TestCompletedSubmissionPrefersPersistedFingerprint(t *testing.T) {
	sheet := newMapperSubmittedSheet(t)
	reconstructed, err := submitport.Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}
	const persisted = "acceptance-time-fingerprint"
	if reconstructed == persisted {
		t.Fatal("test requires persisted fingerprint to differ from reconstructed AnswerSheet")
	}

	completed, err := completedSubmission(sheet, persisted, persisted)
	if err != nil {
		t.Fatalf("completedSubmission() error = %v", err)
	}
	if completed == nil || completed.Sheet != sheet || completed.Fingerprint != persisted {
		t.Fatalf("completedSubmission() = %#v, want persisted acceptance fact", completed)
	}
}

func TestCompletedSubmissionFallsBackForLegacyRowWithoutFingerprint(t *testing.T) {
	sheet := newMapperSubmittedSheet(t)
	want, err := submitport.Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}

	completed, err := completedSubmission(sheet, "", want)
	if err != nil {
		t.Fatalf("completedSubmission() error = %v", err)
	}
	if completed == nil || completed.Fingerprint != want {
		t.Fatalf("completedSubmission() = %#v, want reconstructed legacy fingerprint %q", completed, want)
	}
}

func TestCompletedSubmissionRejectsCandidateDifferentFromPersistedFingerprint(t *testing.T) {
	_, err := completedSubmission(newMapperSubmittedSheet(t), "persisted", "different")
	if !errors.Is(err, submitport.ErrIdempotencyConflict) {
		t.Fatalf("completedSubmission() error = %v, want idempotency conflict", err)
	}
}

func TestCompletedSubmissionRejectsMissingAnswerSheet(t *testing.T) {
	completed, err := completedSubmission(nil, "persisted", "")
	if err == nil || completed != nil {
		t.Fatalf("completedSubmission() = (%#v, %v), want data error", completed, err)
	}
}
