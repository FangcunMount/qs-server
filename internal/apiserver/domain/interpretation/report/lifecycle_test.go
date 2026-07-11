package report

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportLifecycleRetriesWithoutChangingOutcome(t *testing.T) {
	now := time.Unix(100, 0)
	rpt, err := NewPendingInterpretReport(meta.FromUint64(7), meta.FromUint64(9), now)
	if err != nil {
		t.Fatal(err)
	}
	if rpt.Status() != StatusPending || rpt.Attempt() != 0 {
		t.Fatalf("initial lifecycle = %s/%d", rpt.Status(), rpt.Attempt())
	}
	if err := rpt.BeginGenerating(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := rpt.Fail("template failed", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if rpt.Status() != StatusFailed || rpt.Attempt() != 1 || rpt.FailureReason() != "template failed" {
		t.Fatalf("failed lifecycle = %#v", rpt)
	}
	if err := rpt.BeginGenerating(now.Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	generated := NewInterpretReport(rpt.ID(), "model", "M-1", 1, RiskLevelNone, "ok", nil, nil, nil)
	if err := rpt.CompleteFrom(generated, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if rpt.Status() != StatusGenerated || rpt.Attempt() != 2 || rpt.OutcomeID().Uint64() != 9 || rpt.FailureReason() != "" {
		t.Fatalf("generated lifecycle = %#v", rpt)
	}
}
