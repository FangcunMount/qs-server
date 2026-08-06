package report

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// PresentationProfileSource records whether dimension visibility came from the
// report artifact.
type PresentationProfileSource string

const (
	PresentationProfileSourceFrozen         PresentationProfileSource = "frozen"
	PresentationProfileSourceLegacyArtifact PresentationProfileSource = "legacy_artifact_dimensions/v1"
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
	return p.Source == PresentationProfileSourceFrozen ||
		p.Source == PresentationProfileSourceLegacyArtifact
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
