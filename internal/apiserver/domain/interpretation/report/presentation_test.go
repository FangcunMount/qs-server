package report

import "testing"

func TestPresentationProfileConfiguredAcceptsPersistedSourcesOnly(t *testing.T) {
	t.Parallel()

	for _, source := range []PresentationProfileSource{PresentationProfileSourceFrozen, PresentationProfileSourceLegacyArtifact} {
		if !(PresentationProfile{Source: source}).Configured() {
			t.Fatalf("source %q must be configured", source)
		}
	}
	for _, source := range []PresentationProfileSource{"", "legacy", "unknown"} {
		if (PresentationProfile{Source: source}).Configured() {
			t.Fatalf("source %q must be rejected", source)
		}
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
