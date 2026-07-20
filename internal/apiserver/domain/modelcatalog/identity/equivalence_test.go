package identity_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestTypologyAlgorithmsEquivalent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		left, right binding.Algorithm
		want        bool
	}{
		{binding.AlgorithmMBTI, binding.AlgorithmMBTI, true},
		{binding.AlgorithmMBTI, binding.AlgorithmPersonalityTypology, true},
		{binding.AlgorithmPersonalityTypology, binding.AlgorithmSBTI, true},
		{binding.AlgorithmBigFive, binding.AlgorithmPersonalityTypology, true},
		{binding.AlgorithmMBTI, binding.AlgorithmSBTI, false},
		{binding.AlgorithmMBTI, binding.AlgorithmBigFive, false},
		{binding.AlgorithmSPM, binding.AlgorithmPersonalityTypology, false},
	}
	for _, tc := range cases {
		got := identity.TypologyAlgorithmsEquivalent(tc.left, tc.right)
		if got != tc.want {
			t.Fatalf("%s ~ %s = %v, want %v", tc.left, tc.right, got, tc.want)
		}
	}
}

func TestTypologyAlgorithmLookupAlternates(t *testing.T) {
	t.Parallel()
	alts := identity.TypologyAlgorithmLookupAlternates(binding.AlgorithmMBTI)
	if len(alts) != 1 || alts[0] != binding.AlgorithmPersonalityTypology {
		t.Fatalf("mbti alts = %#v", alts)
	}
	alts = identity.TypologyAlgorithmLookupAlternates(binding.AlgorithmPersonalityTypology)
	if len(alts) != 3 {
		t.Fatalf("canonical alts = %#v", alts)
	}
}
