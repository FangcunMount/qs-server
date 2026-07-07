package characterization_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// V1 contract: legacy flat kinds map to stable ExecutionIdentity triples.
func TestV1LegacyKindMapsToExecutionIdentity(t *testing.T) {
	cases := []struct {
		legacy modelcatalog.Kind
		want   evaluation.ExecutionIdentity
	}{
		{modelcatalog.KindScale, evaluation.ExecutionIdentityScaleDefault},
		{modelcatalog.Kind("mbti"), evaluation.ExecutionIdentityMBTI},
		{modelcatalog.Kind("sbti"), evaluation.ExecutionIdentitySBTI},
	}
	for _, tc := range cases {
		got, ok := evaluation.ExecutionIdentityFromLegacyKind(tc.legacy)
		if !ok {
			t.Fatalf("legacy %s: not mapped", tc.legacy)
		}
		if got != tc.want {
			t.Fatalf("legacy %s: got %s, want %s", tc.legacy, got, tc.want)
		}
	}
}

// V1 contract: port ModelRef with legacy kind falls back to ExecutionIdentity mapping.
func TestV1ModelRefExecutionIdentityFromLegacyKind(t *testing.T) {
	ref := evaluationinput.ModelRef{
		Kind: "mbti",
		Code: "MBTI_TEST",
	}
	if got := ref.ExecutionIdentity(); got != evaluation.ExecutionIdentityMBTI {
		t.Fatalf("got %s, want %s", got, evaluation.ExecutionIdentityMBTI)
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
