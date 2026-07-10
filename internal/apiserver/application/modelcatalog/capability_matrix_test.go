package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestAPICatalogCapabilityMatrix(t *testing.T) {
	t.Parallel()

	for _, entry := range catalogKinds {
		entry := entry
		t.Run(string(entry.Kind), func(t *testing.T) {
			t.Parallel()

			apiKind := DomainKindToAPIKind(entry.Kind)
			mapped, ok := APIKindToDomainKind(apiKind)
			if !ok || mapped != entry.Kind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, %v; want %q, true", apiKind, mapped, ok, entry.Kind)
			}
			capability, ok := domain.FamilyCapabilityByKind(entry.Kind)
			if !ok || capability.ExecutionPath == "" {
				t.Fatalf("family capability for %q = %#v, %v", entry.Kind, capability, ok)
			}
		})
	}
}

func TestProductModelRuntimeContractMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		apiKind        string
		productChannel domain.ProductChannel
		kind           domain.Kind
		subKind        domain.SubKind
		algorithm      domain.Algorithm
		family         domain.AlgorithmFamily
		executionPath  domain.ExecutionPath
	}{
		{"scale", KindScale, domain.ProductChannelMedicalScale, domain.KindScale, "", domain.AlgorithmScaleDefault, domain.AlgorithmFamilyFactorScoring, domain.ExecutionPathScaleDescriptor},
		{"typology", KindTypology, domain.ProductChannelTypology, domain.KindTypology, domain.SubKindTypology, domain.AlgorithmMBTI, domain.AlgorithmFamilyFactorClassification, domain.ExecutionPathTypologyDescriptor},
		{"behavioral_rating", KindBehavioralRating, domain.ProductChannelBehaviorAbility, domain.KindBehavioralRating, "", domain.AlgorithmBrief2, domain.AlgorithmFamilyFactorNorm, domain.ExecutionPathBehavioralRatingDescriptor},
		{"cognitive", KindCognitive, domain.ProductChannelBehaviorAbility, domain.KindCognitive, "", domain.AlgorithmSPM, domain.AlgorithmFamilyTaskPerformance, domain.ExecutionPathCognitiveDescriptor},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kind, ok := APIKindToDomainKind(tc.apiKind)
			if !ok || kind != tc.kind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, %v", tc.apiKind, kind, ok)
			}
			if got := domain.DefaultProductChannelFor(tc.kind); got != tc.productChannel {
				t.Fatalf("DefaultProductChannelFor(%q) = %q, want %q", tc.kind, got, tc.productChannel)
			}
			family, ok := domain.AlgorithmFamilyFromIdentity(tc.kind, tc.subKind, tc.algorithm)
			if !ok || family != tc.family {
				t.Fatalf("AlgorithmFamilyFromIdentity(%q,%q,%q) = %q, %v", tc.kind, tc.subKind, tc.algorithm, family, ok)
			}
			capability, ok := domain.FamilyCapabilityByKind(tc.kind)
			if !ok || capability.ExecutionPath != tc.executionPath {
				t.Fatalf("FamilyCapabilityByKind(%q) = %#v, %v", tc.kind, capability, ok)
			}
		})
	}
}
