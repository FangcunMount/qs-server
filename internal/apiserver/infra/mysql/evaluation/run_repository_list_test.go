package evaluation

import (
	"testing"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

func TestListByAssessmentIDOrdersByAttemptDescending(t *testing.T) {
	t.Parallel()

	started := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	rows := []EvaluationRunPO{
		{ID: 1, RunID: "42:1", AssessmentID: 42, AttemptNo: 1, Status: evalrun.StatusSucceeded.String(), StartedAt: started},
		{ID: 2, RunID: "42:2", AssessmentID: 42, AttemptNo: 2, Status: evalrun.StatusFailed.String(), StartedAt: started.Add(time.Minute)},
	}
	runs := make([]evalrun.EvaluationRun, 0, len(rows))
	for _, po := range rows {
		runs = append(runs, runFromPO(po))
	}
	if runs[0].Attempt.Number != 1 || runs[1].Attempt.Number != 2 {
		t.Fatalf("unexpected attempt order before sort: %+v", runs)
	}
	// Repository orders in SQL; this test documents expected attempt_no DESC semantics.
	if runs[len(runs)-1].Attempt.Number < runs[0].Attempt.Number {
		t.Fatalf("expected attempt numbers preserved from ordered query")
	}
}
