package characterization_test

import (
	"context"
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// V1 cross-module contract: behavioral_rating follows submit → worker → split-phase report path.
func TestV1CrossModuleSyncBehavioralRatingSurveySubmitWorkerToInterpretedReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftBehavioralRatingAssessment(t)
	h := buildCharCrossModuleHarness(t, charCrossModuleConfig{
		Assessment: a,
		v1SplitPhaseConfig: v1SplitPhaseConfig{
			Assessment: a,
			Input:      behavioralRatingInputSnapshot(),
			ReportBuilder: interpretationreporting.NewNormProfileReportBuilder(
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
		t.Fatal("expected behavioral_rating report to be saved")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}
