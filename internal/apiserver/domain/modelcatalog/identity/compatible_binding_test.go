package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestCompatibleAlgorithmBindingMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		kind      binding.Kind
		subKind   binding.SubKind
		algorithm binding.Algorithm
		want      bool
	}{
		{name: "scale_default", kind: binding.KindScale, algorithm: binding.AlgorithmScaleDefault, want: true},
		{name: "scale_empty", kind: binding.KindScale, want: true},
		{name: "scale_rejects_brief2", kind: binding.KindScale, algorithm: binding.AlgorithmBrief2, want: false},
		{name: "typology_mbti", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmMBTI, want: true},
		{name: "typology_empty_subkind_draft", kind: binding.KindTypology, algorithm: binding.AlgorithmMBTI, want: true},
		{name: "typology_rejects_trait", kind: binding.KindTypology, subKind: binding.SubKindTrait, algorithm: binding.AlgorithmMBTI, want: false},
		{name: "typology_rejects_spm", kind: binding.KindTypology, subKind: binding.SubKindTypology, algorithm: binding.AlgorithmSPM, want: false},
		{name: "behavioral_brief2", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmBrief2, want: true},
		{name: "behavioral_rejects_raven_spm", kind: binding.KindBehavioralRating, algorithm: binding.AlgorithmSPM, want: false},
		{name: "cognitive_spm", kind: binding.KindCognitive, algorithm: binding.AlgorithmSPM, want: true},
		{name: "cognitive_rejects_brief2", kind: binding.KindCognitive, algorithm: binding.AlgorithmBrief2, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identity.CompatibleAlgorithmBinding(tc.kind, tc.subKind, tc.algorithm)
			if got != tc.want {
				t.Fatalf("CompatibleAlgorithmBinding(%s,%s,%s) = %v, want %v", tc.kind, tc.subKind, tc.algorithm, got, tc.want)
			}
		})
	}
}
