package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// PresentationProfileSource records whether dimension visibility came from the
// report artifact or a one-time legacy read-path fallback.
type PresentationProfileSource string

const (
	PresentationProfileSourceFrozen PresentationProfileSource = "frozen"
	PresentationProfileSourceLegacy PresentationProfileSource = "legacy"
)

// PresentationProfile freezes report-visible factor codes at generation time.
type PresentationProfile struct {
	VisibleFactorCodes []string
	Source             PresentationProfileSource
}

func NewFrozenPresentationProfile(codes []string) PresentationProfile {
	return PresentationProfile{
		VisibleFactorCodes: append([]string(nil), codes...),
		Source:             PresentationProfileSourceFrozen,
	}
}

func (p PresentationProfile) Configured() bool {
	return p.Source == PresentationProfileSourceFrozen || p.Source == PresentationProfileSourceLegacy
}

func (p PresentationProfile) VisibleSet() map[string]bool {
	visible := make(map[string]bool, len(p.VisibleFactorCodes))
	for _, code := range p.VisibleFactorCodes {
		if code != "" {
			visible[code] = true
		}
	}
	return visible
}

// UsesFactorScoreVisibility reports whether a model applies factor-score section
// visibility rather than typology-style dimension presentation.
func UsesFactorScoreVisibility(model ModelIdentity) bool {
	switch model.Kind {
	case string(modelcatalog.KindTypology):
		return false
	case "personality":
		return false
	default:
		return model.Code != ""
	}
}

// LegacyDimensionVisibilityResolver resolves current published factor visibility
// for artifacts that predate frozen presentation profiles.
type LegacyDimensionVisibilityResolver interface {
	VisibleFactorCodes(ctx context.Context, model ModelIdentity) (map[string]bool, bool, error)
}

// ResolvePresentationProfile returns the visibility profile for read projection.
// Frozen artifacts always win; legacy artifacts fall back once through resolver.
func ResolvePresentationProfile(
	ctx context.Context,
	model ModelIdentity,
	stored *PresentationProfile,
	resolver LegacyDimensionVisibilityResolver,
) (PresentationProfile, bool, error) {
	if stored != nil && stored.Source == PresentationProfileSourceFrozen {
		return *stored, true, nil
	}
	if !UsesFactorScoreVisibility(model) {
		return PresentationProfile{}, false, nil
	}
	if resolver == nil {
		return PresentationProfile{}, false, nil
	}
	visible, configured, err := resolver.VisibleFactorCodes(ctx, model)
	if err != nil {
		return PresentationProfile{}, false, err
	}
	if !configured {
		return PresentationProfile{}, false, nil
	}
	codes := make([]string, 0, len(visible))
	for code, ok := range visible {
		if ok && code != "" {
			codes = append(codes, code)
		}
	}
	return PresentationProfile{VisibleFactorCodes: codes, Source: PresentationProfileSourceLegacy}, true, nil
}

func FilterDimensionInterprets(dimensions []DimensionInterpret, visible map[string]bool) []DimensionInterpret {
	if len(dimensions) == 0 {
		return nil
	}
	filtered := make([]DimensionInterpret, 0, len(dimensions))
	for _, dimension := range dimensions {
		if visible[dimension.Code().String()] {
			filtered = append(filtered, dimension)
		}
	}
	return filtered
}

// ComplianceMaskingLayer is an explicit audit-backed overlay for emergency
// content hiding. It must not be implemented as a routine ModelCatalog publish.
type ComplianceMaskingLayer interface {
	Apply(ctx context.Context, assessmentID ID, dimensions []DimensionInterpret) ([]DimensionInterpret, error)
}

func clonePresentationProfile(profile *PresentationProfile) *PresentationProfile {
	if profile == nil {
		return nil
	}
	cloned := &PresentationProfile{
		VisibleFactorCodes: append([]string(nil), profile.VisibleFactorCodes...),
		Source:             profile.Source,
	}
	if cloned.Source == "" {
		return nil
	}
	return cloned
}
