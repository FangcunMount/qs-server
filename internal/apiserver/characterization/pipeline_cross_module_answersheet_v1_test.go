package characterization_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// V1 cross-module contract: survey answersheet.submitted → worker creates+auto-submits assessment →
// assessment.submitted worker evaluates → interpreted report.
func TestV1CrossModuleAnswerSheetSubmittedWorkerToInterpretedReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const answerSheetID uint64 = 6001
	h := buildCharAnswerSheetCrossModuleHarness(t, charAnswerSheetCrossModuleConfig{
		AnswerSheetID: answerSheetID,
	})

	h.runAnswerSheetSubmittedWorker(t, ctx, answerSheetID)
	if !hasStagedEvent(*h.submitStaged, eventcatalog.AssessmentSubmitted) {
		t.Fatalf("submit staged = %#v, want assessment.submitted after auto-submit", *h.submitStaged)
	}

	h.runSubmittedWorker(t, ctx)

	if !h.assessment.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", h.assessment.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected report to be saved after full answersheet pipeline")
	}
	if score := h.assessment.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := h.assessment.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}

// V1 cross-module contract: answersheet path with async split-phase stops at evaluated,
// then assessment.evaluated worker completes report generation.
func TestV1CrossModuleAsyncAnswerSheetSubmittedWorkerEvaluatedToReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	const answerSheetID uint64 = 6001
	h := buildCharAnswerSheetCrossModuleHarness(t, charAnswerSheetCrossModuleConfig{
		AnswerSheetID: answerSheetID,
		Async:         true,
	})

	h.runAnswerSheetSubmittedWorker(t, ctx, answerSheetID)
	h.runSubmittedWorker(t, ctx)

	if !h.assessment.Status().IsEvaluated() {
		t.Fatalf("assessment status after submitted worker = %s, want evaluated", h.assessment.Status())
	}
	if h.reportSaver.saved {
		t.Fatal("report should not be saved before evaluated worker")
	}
	if len(h.evaluateStaged) != 1 || h.evaluateStaged[0].EventType() != eventcatalog.AssessmentEvaluated {
		t.Fatalf("evaluate staged = %#v, want [assessment.evaluated]", h.evaluateStaged)
	}

	h.runEvaluatedWorker(t, ctx)

	if !h.assessment.Status().IsEvaluated() {
		t.Fatalf("assessment status after evaluated worker = %s, want evaluated", h.assessment.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected report to be saved after evaluated worker")
	}
}
