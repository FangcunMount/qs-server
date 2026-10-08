package scheduler

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationconsistency"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

func TestHistoricalSetClassificationPreservesCurrentBusinessChecks(t *testing.T) {
	for _, test := range []struct {
		name    string
		class   evidence.Class
		invalid string
		want    int
	}{{"retired", evidence.RetiredVerified, "", 1}, {"gap", evidence.Unverifiable, "", 2}, {"conflict", evidence.RetiredVerified, "owner conflict", 2}, {"standard-forbidden", evidence.StandardReferenceClass, "", 2}} {
		t.Run(test.name, func(t *testing.T) {
			reader := &consistencyReaderStub{batches: map[uint64]evaluationconsistency.Batch{0: {Items: []evaluationconsistency.AssessmentEvidence{{AssessmentID: 42, Status: "evaluated", HistoricalReferences: []evaluationconsistency.HistoricalReferenceEvidence{{Class: test.class, InvalidReason: test.invalid}}}}, CycleComplete: true}}}
			got, err := NewService(reader).AuditBatch(context.Background(), 0, 1)
			if err != nil || got.Detected != test.want {
				t.Fatal("history suppressed missing canonical Outcome or became standard success", got, err)
			}
		})
	}
}

func TestHistoricalOnlyCommittedBranchKeepsOtherChecksAndSingleGap(t *testing.T) {
	for _, test := range []struct {
		name   string
		class  evidence.Class
		native bool
		status string
		want   int
	}{{"verified", evidence.RetiredVerified, false, "evaluated", 0}, {"gap", evidence.Unverifiable, false, "evaluated", 1}, {"native-corrupt", evidence.RetiredVerified, true, "evaluated", 1}, {"terminal-conflict", evidence.RetiredVerified, false, "failed", 1}} {
		t.Run(test.name, func(t *testing.T) {
			outbox := &evaluationconsistency.CommittedOutboxEvidence{LegacyCanonicalAbsent: true, OutcomeID: "9001", RunID: "42:1", InvalidReason: "canonical outcome lacks classified committed event evidence"}
			if test.native {
				outbox.LegacyCanonicalAbsent = false
				outbox.Class = evidence.StandardReferenceClass
			}
			item := evaluationconsistency.AssessmentEvidence{AssessmentID: 42, Status: test.status, Outcome: &evaluationconsistency.OutcomeEvidence{ID: "9001", RunID: "42:1", ModelKind: "typology"}, Run: &evaluationconsistency.RunEvidence{ID: "42:1", Status: "succeeded"}, Outbox: outbox, CommittedHistory: &evaluationconsistency.CommittedHistoricalEvidence{Class: test.class, OutcomeID: "9001", RunID: "42:1"}, HistoricalReferences: []evaluationconsistency.HistoricalReferenceEvidence{{Class: test.class}}}
			reader := &consistencyReaderStub{batches: map[uint64]evaluationconsistency.Batch{0: {Items: []evaluationconsistency.AssessmentEvidence{item}, CycleComplete: true}}}
			got, err := NewService(reader).AuditBatch(t.Context(), 0, 1)
			if err != nil || got.Detected != test.want {
				t.Fatal("legacy branch changed non-message matrix or duplicated gap", got, err)
			}
		})
	}
}
