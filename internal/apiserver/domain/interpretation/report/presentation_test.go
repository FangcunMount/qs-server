package report

import "testing"

func TestPresentationProfileConfiguredSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     PresentationProfileSource
		configured bool
	}{
		{name: "frozen", source: PresentationProfileSourceFrozen, configured: true},
		{name: "governed legacy artifact", source: PresentationProfileSourceLegacyArtifact, configured: true},
		{name: "retired dynamic legacy", source: "legacy", configured: false},
		{name: "unknown", source: "unknown", configured: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (PresentationProfile{Source: tt.source}).Configured(); got != tt.configured {
				t.Fatalf("Configured() = %v, want %v", got, tt.configured)
			}
		})
	}
}

func TestFilterDimensionInterpretsHonorsFrozenVisibility(t *testing.T) {
	t.Parallel()

	dimensions := []DimensionInterpret{
		NewDimensionInterpret(NewFactorCode("f1"), "F1", 1, nil, RiskLevelLow, "", ""),
		NewDimensionInterpret(NewFactorCode("hidden"), "Hidden", 2, nil, RiskLevelLow, "", ""),
	}
	filtered := FilterDimensionInterprets(dimensions, map[string]bool{"f1": true})
	if len(filtered) != 1 || filtered[0].Code().String() != "f1" {
		t.Fatalf("filtered = %#v", filtered)
	}
}
