package evaluation

import (
	"testing"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

func TestRunPORoundTrip(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	finished := started.Add(2 * time.Second)
	code := evalrun.FailureKindCalculation.String()
	message := "calculation failed"
	traceID := "trace-1"
	original := evalrun.EvaluationRun{
		RunID:        evalrun.ID("run-abc"),
		AssessmentID: 42,
		Attempt: evalrun.Attempt{
			Number: 2,
			Status: evalrun.StatusFailed,
		},
		StartedAt:  started,
		FinishedAt: &finished,
		TraceID:    traceID,
		Failure: &evalrun.Failure{
			Kind:      evalrun.FailureKindCalculation,
			Message:   message,
			Retryable: true,
		},
	}
	po := runToPO(original)
	if po.RunID != "run-abc" || po.AssessmentID != 42 || po.AttemptNo != 2 {
		t.Fatalf("unexpected po: %+v", po)
	}
	if po.ErrorCode == nil || *po.ErrorCode != code {
		t.Fatalf("error code = %v, want %s", po.ErrorCode, code)
	}
	if po.TraceID == nil || *po.TraceID != traceID {
		t.Fatalf("trace id = %v, want %s", po.TraceID, traceID)
	}
	if !po.Retryable {
		t.Fatal("retryable should be true")
	}

	roundTrip := runFromPO(*po)
	if roundTrip.RunID != original.RunID {
		t.Fatalf("run id = %s, want %s", roundTrip.RunID, original.RunID)
	}
	if roundTrip.Attempt.Number != original.Attempt.Number || roundTrip.Attempt.Status != original.Attempt.Status {
		t.Fatalf("attempt = %+v, want %+v", roundTrip.Attempt, original.Attempt)
	}
	if roundTrip.Failure == nil || roundTrip.Failure.Message != message || !roundTrip.Failure.Retryable {
		t.Fatalf("failure = %+v", roundTrip.Failure)
	}
}
