package interpretation

import (
	"context"
	"errors"
	"testing"
	"time"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestReportFailureDoesNotModifyEvaluationFacts(t *testing.T) {
	record := reportOutcomeRecord(t)
	payloadBefore := append([]byte(nil), record.Payload()...)
	assessmentBefore := evaluatedAssessmentForReport(t)
	scoreBefore := *assessmentBefore.TotalScore()
	run := evalrun.NewEvaluationRunWithAttempt(assessmentBefore.ID().Uint64(), 1)
	if err := run.Start(time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	if err := run.Succeed(time.Unix(200, 0)); err != nil {
		t.Fatal(err)
	}
	outcomes := &outcomeRepoForReport{record: record}
	states := &reportStateStoreStub{}
	svc := NewOutcomeReportService(outcomes, states, &failThenGenerate{}, &durableReportSaverStub{})

	failed, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if !errors.Is(err, errReportBuild) {
		t.Fatalf("generation error = %v, want report failure", err)
	}
	if failed.Status() != domainreport.ReportStatusFailed {
		t.Fatalf("report status = %s, want failed", failed.Status())
	}
	if string(record.Payload()) != string(payloadBefore) {
		t.Fatal("report failure modified persisted EvaluationOutcome")
	}
	if !assessmentBefore.Status().IsEvaluated() || assessmentBefore.TotalScore() == nil || *assessmentBefore.TotalScore() != scoreBefore {
		t.Fatalf("report failure modified assessment facts: status=%s total_score=%v", assessmentBefore.Status(), assessmentBefore.TotalScore())
	}
	if run.Attempt.Status != evalrun.StatusSucceeded {
		t.Fatalf("report failure modified evaluation run: status=%s", run.Attempt.Status)
	}
}

func TestReportRetryReadsEvaluationOutcomeWithoutEvaluator(t *testing.T) {
	record := reportOutcomeRecord(t)
	outcomes := &outcomeRepoForReport{record: record}
	generator := &failThenGenerate{}
	svc := NewOutcomeReportService(outcomes, &reportStateStoreStub{}, generator, &durableReportSaverStub{})

	_, _ = svc.GenerateByOutcomeID(context.Background(), record.ID())
	generated, err := svc.GenerateByOutcomeID(context.Background(), record.ID())
	if err != nil {
		t.Fatalf("retry generation: %v", err)
	}
	if generated.Status() != domainreport.ReportStatusGenerated || outcomes.reads != 2 || generator.calls != 2 {
		t.Fatalf("status=%s outcome_reads=%d generator_calls=%d", generated.Status(), outcomes.reads, generator.calls)
	}
}

func evaluatedAssessmentForReport(t *testing.T) *assessment.Assessment {
	t.Helper()
	model := assessment.NewScaleEvaluationModelRef(0, meta.NewCode("S-1"), "1.0.0", "Scale")
	a, err := assessment.NewAssessment(
		11,
		testee.NewID(8),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(7)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7)),
		assessment.WithEvaluationModel(model),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	execution := domainoutcome.NewExecution(
		evaloutcome.ModelRefFromAssessment(model), domainoutcome.Summary{PrimaryLabel: "low"}, domainoutcome.Detail{Kind: modelcatalog.KindScale},
	)
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
	if err := a.ApplyScoringProjection(evaloutcome.ScoringProjectionFromExecution(execution)); err != nil {
		t.Fatalf("ApplyScoringProjection: %v", err)
	}
	return a
}
