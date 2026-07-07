package characterization_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// V1 contract: only scale flat kind still maps via ExecutionIdentityFromLegacyKind (R29-C1).
func TestV1LegacyKindMapsToExecutionIdentity(t *testing.T) {
	got, ok := evaluation.ExecutionIdentityFromLegacyKind(modelcatalog.KindScale)
	if !ok {
		t.Fatal("legacy scale: not mapped")
	}
	if got != evaluation.ExecutionIdentityScaleDefault {
		t.Fatalf("legacy scale: got %s, want %s", got, evaluation.ExecutionIdentityScaleDefault)
	}
	for _, legacy := range []modelcatalog.Kind{modelcatalog.Kind("mbti"), modelcatalog.Kind("sbti")} {
		if _, ok := evaluation.ExecutionIdentityFromLegacyKind(legacy); ok {
			t.Fatalf("legacy %s: should not map after R29-C1", legacy)
		}
	}
}

// V1 contract: flat mbti ModelRef without v2 fields no longer auto-maps; use v2 triple instead.
func TestV1ModelRefExecutionIdentityFromLegacyKind(t *testing.T) {
	ref := evaluationinput.ModelRef{
		Kind: "mbti",
		Code: "MBTI_TEST",
	}
	got := ref.ExecutionIdentity()
	want := evaluation.ExecutionIdentity{Kind: modelcatalog.Kind("mbti")}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	v2 := evaluationinput.ModelRef{
		Kind:      "personality",
		SubKind:   "typology",
		Algorithm: "mbti",
		Code:      "MBTI_TEST",
	}
	if got := v2.ExecutionIdentity(); got != evaluation.ExecutionIdentityMBTI {
		t.Fatalf("v2 ref got %s, want %s", got, evaluation.ExecutionIdentityMBTI)
	}
}

// V1 contract: port ModelSnapshot carries v2 identity fields for routing.
func TestV1ModelSnapshotCarriesV2IdentityFields(t *testing.T) {
	snapshot := evaluationinput.NewMBTIModelSnapshot(mbtiINTJModel())
	if snapshot.SubKind != "typology" || snapshot.Algorithm != "mbti" {
		t.Fatalf("snapshot identity = sub:%s algo:%s", snapshot.SubKind, snapshot.Algorithm)
	}
	ref := snapshot.ModelRef()
	if got := ref.ExecutionIdentity().String(); got != "personality/typology/mbti" {
		t.Fatalf("ref identity = %s, want personality/typology/mbti", got)
	}
}

// V1 contract: assessment EvaluationModelRef preserves legacy kind routing when v2 fields absent.
func TestV1AssessmentModelRefExecutionIdentityFromLegacyKind(t *testing.T) {
	ref := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindScale,
		meta.NewCode("S-001"),
		"1.0.0",
		"Scale",
	)
	if ref.ExecutionIdentity() != evaluation.ExecutionIdentityScaleDefault {
		t.Fatalf("got %s, want %s", ref.ExecutionIdentity(), evaluation.ExecutionIdentityScaleDefault)
	}
}
