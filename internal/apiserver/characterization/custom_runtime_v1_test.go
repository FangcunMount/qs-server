package characterization_test

import (
	"context"
	"testing"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/rendering"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
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
	if executor.Key() != evaluation.ExecutionIdentityPersonalityTypology {
		t.Fatalf("executor key = %s, want configured typology key", executor.Key())
	}

	reportBuilder := typologyreporting.NewTypologyBuilder()
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
	detail, ok := outcome.Detail.Payload.(outcometypology.PersonalityTypeDetail)
	if !ok {
		t.Fatalf("detail type = %T, want PersonalityTypeDetail", outcome.Detail.Payload)
	}
	if detail.TypeCode != "INTJ" {
		t.Fatalf("TypeCode = %s, want INTJ", detail.TypeCode)
	}
	if detail.MatchPercent != 40 {
		t.Fatalf("MatchPercent = %.2f, want 40", detail.MatchPercent)
	}

	report := buildPreviewReport(t, reportBuilder, previewOutcome(t, assessment, snapshot, outcome, evaluationfact.RuntimeIdentity{}))
	if report.Conclusion() == "" {
		t.Fatal("expected non-empty report conclusion")
	}
	if extra := report.ModelExtra(); extra == nil || extra.TypeCode != "INTJ" {
		t.Fatalf("ModelExtra = %#v, want INTJ", extra)
	}
}
