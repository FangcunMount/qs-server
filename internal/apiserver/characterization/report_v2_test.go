package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

// V2 contract: scale report builder projects model/primary_score/level and Mongo preserves them.
func TestV2ScaleReportProjectsOutcomeSummaryFields(t *testing.T) {
	a := submittedScaleAssessment(t)
	snapshot := scaleInputSnapshot()
	execution, err := factorscoring.NewExecutor(nil).Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	report := buildLegacyReport(t, interpretationreporting.NewFactorScoringReportBuilder(domainreport.NewDefaultReportBuilder(nil)), evaloutcome.Outcome{Assessment: a, Input: snapshot, Execution: execution})
	if report.Model().Kind != "scale" || report.Model().Algorithm != "scale_default" {
		t.Fatalf("model = %#v", report.Model())
	}
	if report.PrimaryScore() == nil || report.PrimaryScore().Kind != domainreport.ScoreKindRawTotal || report.PrimaryScore().Value != 5 {
		t.Fatalf("primary score = %#v", report.PrimaryScore())
	}
	if report.Level() == nil || report.Level().Code != "low" {
		t.Fatalf("level = %#v", report.Level())
	}

}
