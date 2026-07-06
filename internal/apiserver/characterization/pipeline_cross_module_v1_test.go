package characterization_test

import (
	"context"
	"testing"

	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// V1 cross-module contract: survey completes → assessment submit stages assessment.submitted →
// worker calls EvaluateAssessment → split-phase execute persists score + report → interpreted.
func TestV1CrossModuleSyncScaleSurveySubmitWorkerToInterpretedReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftScaleAssessment(t)
	h := buildCharCrossModuleHarness(t, scaleCrossModuleConfig(t, a, false))

	h.submitAssessment(t, ctx)
	if !hasStagedEvent(*h.submitStaged, eventcatalog.AssessmentSubmitted) {
		t.Fatalf("submit staged = %#v, want assessment.submitted", *h.submitStaged)
	}

	h.runSubmittedWorker(t, ctx)

	if !a.Status().IsInterpreted() {
		t.Fatalf("assessment status = %s, want interpreted", a.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected report to be saved after worker-driven evaluation")
	}
	if score := a.TotalScore(); score == nil || *score != 5 {
		t.Fatalf("total score = %v, want 5", score)
	}
	if risk := a.RiskLevel(); risk == nil || *risk != assessment.RiskLevelLow {
		t.Fatalf("risk level = %v, want low", risk)
	}
}

// V1 cross-module contract: async split-phase stops at evaluated after worker submit handler,
// then assessment.evaluated worker handler completes report generation.
func TestV1CrossModuleAsyncScaleSurveySubmitWorkerEvaluatedToReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftScaleAssessment(t)
	h := buildCharCrossModuleHarness(t, scaleCrossModuleConfig(t, a, true))

	h.submitAssessment(t, ctx)
	h.runSubmittedWorker(t, ctx)

	if !a.Status().IsEvaluated() {
		t.Fatalf("assessment status after submitted worker = %s, want evaluated", a.Status())
	}
	if h.reportSaver.saved {
		t.Fatal("report should not be saved before evaluated worker")
	}
	if len(h.evaluateStaged) != 1 || h.evaluateStaged[0].EventType() != eventcatalog.AssessmentEvaluated {
		t.Fatalf("evaluate staged = %#v, want [assessment.evaluated]", h.evaluateStaged)
	}

	h.runEvaluatedWorker(t, ctx)

	if !a.Status().IsInterpreted() {
		t.Fatalf("assessment status after evaluated worker = %s, want interpreted", a.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected report to be saved after evaluated worker")
	}
}

// V1 cross-module contract: personality typology follows the same submit → worker → report path.
func TestV1CrossModuleSyncMBTISurveySubmitWorkerToInterpretedReport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	a := draftMBTIAssessment(t)
	reportBuilder, err := typologyeval.NewReportBuilder(modelcatalog.AlgorithmMBTI)
	if err != nil {
		t.Fatalf("NewReportBuilder: %v", err)
	}
	h := buildCharCrossModuleHarness(t, charCrossModuleConfig{
		Assessment: a,
		v1SplitPhaseConfig: v1SplitPhaseConfig{
			Assessment:    a,
			Input:         mbtiInputSnapshot(),
			ReportBuilder: reportBuilder,
		},
	})

	h.submitAssessment(t, ctx)
	h.runSubmittedWorker(t, ctx)

	if !a.Status().IsInterpreted() {
		t.Fatalf("assessment status = %s, want interpreted", a.Status())
	}
	if !h.reportSaver.saved {
		t.Fatal("expected MBTI report to be saved")
	}
	if summary := a.ResultSummary(); summary == nil || summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %v, want INTJ", summary)
	}
}

func draftMBTIAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI_TEST"),
		"1.0.0",
		"MBTI 测试",
	)
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8002),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("MBTI_TEST"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6002)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7002)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	return a
}
