package characterization_test

import (
	"context"
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// V1 cross-module contract: cognitive follows submit → worker → split-phase report path.
func TestV1CrossModuleSyncCognitiveSurveySubmitWorkerToInterpretedReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftCognitiveAssessment(t)
	h := buildCharCrossModuleHarness(t, charCrossModuleConfig{
		Assessment: a,
		v1SplitPhaseConfig: v1SplitPhaseConfig{
			Assessment: a,
			Input:      cognitiveInputSnapshot(),
			ReportBuilder: interpretationreporting.NewTaskPerformanceReportBuilder(
				domainreport.NewDefaultInterpretReportBuilder(nil),
			),
		},
	})

	h.submitAssessment(t, ctx)
	h.runSubmittedWorker(t, ctx)

	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", a.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected cognitive report to be saved")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}
