package characterization_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
)

func TestV1CognitiveExecuteAndReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftCognitiveAssessment(t)
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()

	svc, reportSaver := buildV1SplitPhaseExecuteService(t, v1SplitPhaseConfig{
		Assessment: a,
		Input:      cognitiveInputSnapshot(),
		ReportBuilder: interpretationreporting.NewTaskPerformanceBuilder(
			domainreport.NewDefaultReportBuilder(nil),
		),
	})
	if err := svc.Evaluate(ctx, a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", a.Status())
	}
	if !reportSaver.saved {
		t.Fatal("expected cognitive report to be saved")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}
