package report

import (
	"context"
	"testing"
)

func TestResolvePresentationProfilePrefersFrozenArtifact(t *testing.T) {
	t.Parallel()

	model := ModelIdentity{Kind: "scale", Code: "scl-1"}
	frozen := NewFrozenPresentationProfile([]string{"f1", "f2"})
	legacy := stubLegacyResolver{visible: map[string]bool{"hidden": true}}

	got, configured, err := ResolvePresentationProfile(context.Background(), model, &frozen, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !configured || got.Source != PresentationProfileSourceFrozen {
		t.Fatalf("profile = %#v configured=%v", got, configured)
	}
	if len(got.VisibleFactorCodes) != 2 || got.VisibleFactorCodes[0] != "f1" {
		t.Fatalf("visible codes = %#v", got.VisibleFactorCodes)
	}
}

func TestResolvePresentationProfileUsesLegacyFallbackOnce(t *testing.T) {
	t.Parallel()

	model := ModelIdentity{Kind: "scale", Code: "scl-1"}
	legacy := stubLegacyResolver{visible: map[string]bool{"f1": true}, configured: true}

	got, configured, err := ResolvePresentationProfile(context.Background(), model, nil, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !configured || got.Source != PresentationProfileSourceLegacy {
		t.Fatalf("profile = %#v configured=%v", got, configured)
	}
	if len(got.VisibleFactorCodes) != 1 || got.VisibleFactorCodes[0] != "f1" {
		t.Fatalf("visible codes = %#v", got.VisibleFactorCodes)
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

type stubLegacyResolver struct {
	visible    map[string]bool
	configured bool
}

func (s stubLegacyResolver) VisibleFactorCodes(context.Context, ModelIdentity) (map[string]bool, bool, error) {
	return s.visible, s.configured, nil
}
