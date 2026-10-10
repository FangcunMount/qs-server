package scheduler

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func TestPureAuditSummarySeparatesHistoricalGapsAndRetainsDrifts(t *testing.T) {
	batch := evaluationconsistency.Batch{Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 1, Status: "submitted", HistoricalReferences: []evaluationconsistency.HistoricalReferenceEvidence{{Class: evidence.Unverifiable}}}, {AssessmentID: 2, Status: "evaluated"}}, NextCursor: 2, CycleComplete: true}
	r := SummarizeReadOnlyBatch(batch, time.Now())
	if r.Scanned != 2 || r.HistoricalGaps != 1 || r.Detected <= r.HistoricalGaps || r.NextCursor != 2 || !r.CycleComplete {
		t.Fatal("historical gap lost or hid an actionable consistency drift", r)
	}
}
