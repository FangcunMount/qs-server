package assessmentmodel

import (
	"fmt"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestMergeAlgorithmSetsPreservesMoreThanPageLimit(t *testing.T) {
	v2 := make([]domain.Algorithm, 0, 150)
	for i := 0; i < 150; i++ {
		v2 = append(v2, domain.Algorithm(fmt.Sprintf("algo_%03d", i)))
	}
	got := mergeAlgorithmSets(v2, []domain.Algorithm{domain.AlgorithmMBTI})
	if len(got) != 151 {
		t.Fatalf("algorithm count = %d, want 151", len(got))
	}
}

func TestMergeAlgorithmSetsDedupesLegacyAndV2(t *testing.T) {
	got := mergeAlgorithmSets(
		[]domain.Algorithm{domain.AlgorithmMBTI, domain.AlgorithmBigFive},
		[]domain.Algorithm{domain.AlgorithmMBTI, domain.AlgorithmSBTI},
	)
	if len(got) != 3 {
		t.Fatalf("algorithms = %#v, want 3 distinct", got)
	}
}
