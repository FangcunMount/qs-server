package rendering

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreporttemplate "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/reporttemplate"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// BuiltinReleaseManifests returns the complete immutable release catalog for
// every template implementation registered in the current binary.
func BuiltinReleaseManifests() ([]domainreporttemplate.ReleaseManifest, error) {
	registry, err := NewRegistry(DefaultBuilders(nil)...)
	if err != nil {
		return nil, err
	}
	route := func(decisionKind modelcatalog.DecisionKind, adapterKey string) (domainreporttemplate.ManifestRoute, error) {
		builder, err := registry.ResolveByMechanism(Key{
			DecisionKind: decisionKind, ReportType: policy.ReportTypeStandard, TemplateVersion: policy.TemplateVersionV1,
		})
		if err != nil {
			return domainreporttemplate.ManifestRoute{}, err
		}
		return domainreporttemplate.ManifestRoute{
			DecisionKind: decisionKind, BuilderIdentity: builder.BuilderIdentity(),
			ContentSchemaVersion: builder.ContentSchemaVersion(), AdapterKey: adapterKey,
		}, nil
	}
	routes := func(adapterKey string, decisionKinds ...modelcatalog.DecisionKind) ([]domainreporttemplate.ManifestRoute, error) {
		result := make([]domainreporttemplate.ManifestRoute, 0, len(decisionKinds))
		for _, decisionKind := range decisionKinds {
			item, err := route(decisionKind, adapterKey)
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		}
		return result, nil
	}

	type manifestSpec struct {
		templateID   string
		adapterKey   string
		decisionKind []modelcatalog.DecisionKind
	}
	specs := []manifestSpec{
		{templateID: "standard", decisionKind: []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindScoreRange,
			modelcatalog.DecisionKindNormLookup,
			modelcatalog.DecisionKindAbilityLevel,
		}},
		{templateID: "mbti", adapterKey: "personality_type", decisionKind: []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindPoleComposition,
			modelcatalog.DecisionKindNearestPattern,
			modelcatalog.DecisionKindDominantFactor,
		}},
		{templateID: "sbti", adapterKey: "personality_type", decisionKind: []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindPoleComposition,
			modelcatalog.DecisionKindNearestPattern,
			modelcatalog.DecisionKindDominantFactor,
		}},
		{templateID: "bigfive", adapterKey: "trait_profile", decisionKind: []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindTraitProfile,
		}},
		{templateID: "enneagram", adapterKey: "trait_profile", decisionKind: []modelcatalog.DecisionKind{
			modelcatalog.DecisionKindTraitProfile,
		}},
	}

	manifests := make([]domainreporttemplate.ReleaseManifest, 0, len(specs))
	for _, spec := range specs {
		manifestRoutes, err := routes(spec.adapterKey, spec.decisionKind...)
		if err != nil {
			return nil, fmt.Errorf("resolve report template manifest %s: %w", spec.templateID, err)
		}
		manifest, err := domainreporttemplate.NewReleaseManifest(
			spec.templateID, policy.TemplateVersionV1, policy.ReportTypeStandard, manifestRoutes,
		)
		if err != nil {
			return nil, fmt.Errorf("build report template manifest %s: %w", spec.templateID, err)
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}
