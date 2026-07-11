package characterization_test

import (
	"context"
	"testing"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// V1 contract: production split-phase wiring scores first, then persists report,
// ending with interpreted assessment status.
func TestV1SplitPhasePipelineScaleSubmitToInterpretedOutcome(t *testing.T) {
	a := submittedScaleAssessment(t)
	svc, reportSaver := buildV1SplitPhaseExecuteService(t, v1SplitPhaseConfig{
		Assessment: a,
		Input:      scaleInputSnapshot(),
		ReportBuilder: interpretationreporting.NewFactorScoringReportBuilder(
			domainreport.NewDefaultInterpretReportBuilder(nil),
		),
	})
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", a.Status())
	}
	if !reportSaver.saved {
		t.Fatal("expected report to be saved in interpretation phase")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}

// V1 contract: personality split-phase path completes with interpreted status and type label.
func TestV1SplitPhasePipelineMBTISubmitToInterpretedOutcome(t *testing.T) {
	a := submittedMBTIAssessment(t)
	reportBuilder, err := typologyreporting.NewReportBuilder(modelcatalog.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewReportBuilder: %v", err)
	}
	svc, reportSaver := buildV1SplitPhaseExecuteService(t, v1SplitPhaseConfig{
		Assessment:    a,
		Input:         mbtiInputSnapshot(),
		ReportBuilder: reportBuilder,
	})
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status = %s, want evaluated", a.Status())
	}
	if !reportSaver.saved {
		t.Fatal("expected report to be saved in interpretation phase")
	}
	if summary := a.ResultSummary(); summary == nil || summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %v, want INTJ", summary)
	}
}

// V1 contract: async split-phase stops after scoring, stages assessment.evaluated,
// and GenerateReport completes interpretation from stored snapshot.
func TestV1SplitPhaseAsyncScaleStopsAtEvaluatedThenGenerateReport(t *testing.T) {
	a := submittedScaleAssessment(t)
	var staged []string
	svc, reportSaver := buildV1SplitPhaseExecuteService(t, v1SplitPhaseConfig{
		Assessment: a,
		Input:      scaleInputSnapshot(),
		ReportBuilder: interpretationreporting.NewFactorScoringReportBuilder(
			domainreport.NewDefaultInterpretReportBuilder(nil),
		),
		Async: true,
		StageEvaluated: func(_ context.Context, events ...event.DomainEvent) error {
			for _, evt := range events {
				staged = append(staged, evt.EventType())
			}
			return nil
		},
	})
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status after Evaluate = %s, want evaluated", a.Status())
	}
	if reportSaver.saved {
		t.Fatal("report should not be saved before async GenerateReport")
	}
	if len(staged) != 1 || staged[0] != eventcatalog.AssessmentEvaluated {
		t.Fatalf("staged events = %#v, want [%q]", staged, eventcatalog.AssessmentEvaluated)
	}

	if err := svc.GenerateReport(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("GenerateReport: %v", err)
	}
	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status after GenerateReport = %s, want evaluated", a.Status())
	}
	if !reportSaver.saved {
		t.Fatal("expected report to be saved after GenerateReport")
	}
}
