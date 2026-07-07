package characterization_test

import (
	"context"
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

func TestV1BehavioralRatingExecuteAndReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftBehavioralRatingAssessment(t)
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	a.ClearEvents()

	svc, reportSaver := buildV1SplitPhaseExecuteService(t, v1SplitPhaseConfig{
		Assessment: a,
		Input:      behavioralRatingInputSnapshot(),
		ReportBuilder: interpretationreporting.NewNormProfileReportBuilder(
			domainreport.NewDefaultInterpretReportBuilder(nil),
		),
	})
	if err := svc.Evaluate(ctx, a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !a.Status().IsInterpreted() {
		t.Fatalf("assessment status = %s, want interpreted", a.Status())
	}
	if !reportSaver.saved {
		t.Fatal("expected behavioral_rating report to be saved")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}
