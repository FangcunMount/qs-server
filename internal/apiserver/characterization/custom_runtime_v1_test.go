package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_classification"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/factor_classification/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Phase-2 acceptance: a non-MBTI/SBTI/BigFive model runs through the configured typology
// executor and report builder using only explicit payload.runtime — no new module registration.
func TestV2CustomRuntimeTypologyRunsWithoutNewModuleRegistration(t *testing.T) {
	t.Parallel()

	executor, err := typologyeval.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	if executor.Key() != evaluation.EvaluatorKeyPersonalityTypology {
		t.Fatalf("executor key = %s, want configured typology key", executor.Key())
	}

	reportBuilder, err := typologyeval.NewConfiguredReportBuilder()
	if err != nil {
		t.Fatalf("NewConfiguredReportBuilder: %v", err)
	}
	if reportBuilder.Key() != evaluation.EvaluatorKeyPersonalityTypology {
		t.Fatalf("report builder key = %s, want configured typology key", reportBuilder.Key())
	}

	assessment := submittedCustomRuntimeAssessment(t)
	snapshot := customRuntimeInputSnapshot()
	typologyPayload, ok := snapshot.ModelPayload.(evaluationinput.TypologyModelPayload)
	if !ok || typologyPayload.Payload == nil {
		t.Fatal("expected typology model payload")
	}
	if typologyPayload.Payload.Algorithm != "" {
		t.Fatalf("payload algorithm = %q, want empty for explicit runtime model", typologyPayload.Payload.Algorithm)
	}

	outcome, err := executor.Execute(context.Background(), evaluationexecute.ExecutionInput{
		Assessment: assessment,
		Input:      snapshot,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	detail, ok := outcome.Detail.Payload.(evaluationtypology.PersonalityTypeDetail)
	if !ok {
		t.Fatalf("detail type = %T, want PersonalityTypeDetail", outcome.Detail.Payload)
	}
	if detail.TypeCode != "INTJ" {
		t.Fatalf("TypeCode = %s, want INTJ", detail.TypeCode)
	}
	if detail.MatchPercent != 40 {
		t.Fatalf("MatchPercent = %.2f, want 40", detail.MatchPercent)
	}

	report, err := reportBuilder.Build(context.Background(), evaloutcome.Outcome{
		Assessment: assessment,
		Input:      snapshot,
		Execution:  outcome,
	})
	if err != nil {
		t.Fatalf("Build report: %v", err)
	}
	if report.Conclusion() == "" {
		t.Fatal("expected non-empty report conclusion")
	}
	if extra := report.ModelExtra(); extra == nil || extra.TypeCode != "INTJ" {
		t.Fatalf("ModelExtra = %#v, want INTJ", extra)
	}
}
