package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/factor_scoring"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
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
	report, err := interpretationreporting.NewScaleReportBuilder(domainreport.NewDefaultInterpretReportBuilder(nil)).
		Build(context.Background(), evaloutcome.Outcome{Assessment: a, Input: snapshot, Execution: execution})
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if report.Model().Kind != "scale" || report.Model().Algorithm != "scale_default" {
		t.Fatalf("model = %#v", report.Model())
	}
	if report.PrimaryScore() == nil || report.PrimaryScore().Kind != domainreport.ScoreKindRawTotal || report.PrimaryScore().Value != 5 {
		t.Fatalf("primary score = %#v", report.PrimaryScore())
	}
	if report.Level() == nil || report.Level().Code != "low" {
		t.Fatalf("level = %#v", report.Level())
	}

	po := mongoevaluation.NewReportMapper().ToPO(report, 8001)
	if po.Model == nil || po.PrimaryScore == nil || po.Level == nil {
		t.Fatalf("mongo v2 fields missing: model=%#v primary=%#v level=%#v", po.Model, po.PrimaryScore, po.Level)
	}
	roundTrip := mongoevaluation.NewReportMapper().ToDomain(po)
	if roundTrip.PrimaryScore().Value != 5 || roundTrip.Level().Code != "low" {
		t.Fatalf("round trip summary = primary:%#v level:%#v", roundTrip.PrimaryScore(), roundTrip.Level())
	}
}
