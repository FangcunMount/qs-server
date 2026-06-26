package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	mongoevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/evaluation"
)

// V2 contract: scale report builder projects model/primary_score/level and Mongo preserves them.
func TestV2ScaleReportProjectsOutcomeSummaryFields(t *testing.T) {
	a := submittedScaleAssessment(t)
	snapshot := scaleInputSnapshot()
	execution, err := evaluationscale.NewExecutor(nil).Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: a,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	result := execution.ToEvaluationResult()

	report, err := evaluationresult.NewScaleReportBuilder(domainreport.NewDefaultInterpretReportBuilder(nil)).
		Build(context.Background(), evaluationresult.NewOutcomeFromLegacyResult(a, snapshot, result))
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
