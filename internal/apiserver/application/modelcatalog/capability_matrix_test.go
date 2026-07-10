package modelcatalog

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func modelFamilyRegistryOptions() []option.RegisteredOption {
	out := make([]option.RegisteredOption, 0)
	for _, entry := range option.DefaultRegistry().RegisteredOptions() {
		if !entry.IsProductChannel() {
			out = append(out, entry)
		}
	}
	return out
}

func TestAPICatalogCapabilityMatrix(t *testing.T) {
	t.Parallel()

	registry := option.DefaultRegistry()
	for _, entry := range modelFamilyRegistryOptions() {
		entry := entry
		t.Run(entry.APIKind, func(t *testing.T) {
			t.Parallel()

			mapped, ok := APIKindToDomainKind(entry.APIKind)
			if !ok || mapped != entry.Kind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, %v; want %q, true", entry.APIKind, mapped, ok, entry.Kind)
			}
			if got := DomainKindToAPIKind(entry.Kind); got != entry.APIKind {
				t.Fatalf("DomainKindToAPIKind(%q) = %q, want %q", entry.Kind, got, entry.APIKind)
			}
			registered, ok := registry.ByAPIKind(entry.APIKind)
			if !ok || registered.OptionsEnabled != entry.OptionsEnabled {
				t.Fatalf("registry option for %q = %#v, %v", entry.APIKind, registered, ok)
			}
			capability, ok := domain.FamilyCapabilityByKind(entry.Kind)
			if !ok || capability.CreateSupported != entry.Operations.CreateSupported {
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
		{"scale", KindMedicalScale, domain.ProductChannelMedicalScale, domain.KindScale, "", domain.AlgorithmScaleDefault, domain.AlgorithmFamilyFactorScoring, domain.ExecutionPathScaleDescriptor},
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
