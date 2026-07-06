package characterization_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// V1 contract: legacy flat kinds map to stable v2 EvaluatorKey triples.
func TestV1LegacyKindMapsToEvaluatorKey(t *testing.T) {
	cases := []struct {
		legacy modelcatalog.Kind
		want   evaluation.EvaluatorKey
	}{
		{modelcatalog.KindScale, evaluation.EvaluatorKeyScaleDefault},
		{modelcatalog.KindMBTIMigration, evaluation.EvaluatorKeyMBTI},
		{modelcatalog.KindSBTIMigration, evaluation.EvaluatorKeySBTI},
	}
	for _, tc := range cases {
		got, ok := evaluation.EvaluatorKeyFromLegacyKind(tc.legacy)
		if !ok {
			t.Fatalf("legacy %s: not mapped", tc.legacy)
		}
		if got != tc.want {
			t.Fatalf("legacy %s: got %s, want %s", tc.legacy, got, tc.want)
		}
	}
}

// V1 contract: port ModelRef with legacy kind falls back to EvaluatorKey mapping.
func TestV1ModelRefEvaluatorKeyFromLegacyKind(t *testing.T) {
	ref := evaluationinput.ModelRef{
		Kind: evaluationinput.EvaluationModelKindMBTIMigration,
		Code: "MBTI_TEST",
	}
	if got := ref.EvaluatorKey(); got != evaluation.EvaluatorKeyMBTI {
		t.Fatalf("got %s, want %s", got, evaluation.EvaluatorKeyMBTI)
	}
}

// V1 contract: port ModelSnapshot carries v2 identity fields for routing.
func TestV1ModelSnapshotCarriesV2IdentityFields(t *testing.T) {
	snapshot := evaluationinput.NewMBTIModelSnapshot(mbtiINTJModel())
	if snapshot.SubKind != "typology" || snapshot.Algorithm != "mbti" {
		t.Fatalf("snapshot identity = sub:%s algo:%s", snapshot.SubKind, snapshot.Algorithm)
	}
	ref := snapshot.ModelRef()
	// Port ModelRef keeps legacy kind field; execute path uses assessment ref legacy mapping instead.
	if got := ref.EvaluatorKey().String(); got != "personality/typology/mbti" {
		t.Fatalf("ref key = %s, want personality/typology/mbti", got)
	}
}

// V1 contract: assessment EvaluationModelRef preserves legacy kind routing when v2 fields absent.
func TestV1AssessmentModelRefEvaluatorKeyFromLegacyKind(t *testing.T) {
	ref := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindScale,
		meta.NewCode("S-001"),
		"1.0.0",
		"Scale",
	)
	if ref.EvaluatorKey() != evaluation.EvaluatorKeyScaleDefault {
		t.Fatalf("got %s, want %s", ref.EvaluatorKey(), evaluation.EvaluatorKeyScaleDefault)
	}
}
