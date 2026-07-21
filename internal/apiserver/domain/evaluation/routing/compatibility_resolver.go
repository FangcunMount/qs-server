package evaluation

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// CompatibilityResolver is the single entry for legacy RuntimeIdentity derivation (EV-R008).
// Frozen routes must never enter this resolver; use DescriptorKeyFromRoute which fail-closes first.
type CompatibilityResolver struct{}

// NewCompatibilityResolver returns the shared legacy RuntimeIdentity resolver.
func NewCompatibilityResolver() CompatibilityResolver {
	return CompatibilityResolver{}
}

// EnrichLegacyRoute fills draft payload format for incomplete historical routes.
// Call only when the route is not HasFrozenRuntime().
func (CompatibilityResolver) EnrichLegacyRoute(route ModelRoute) ModelRoute {
	if route.HasFrozenRuntime() {
		return route
	}
	if route.PayloadFormat == "" {
		route.PayloadFormat = modelcatalog.DraftPayloadFormatForModel(route.Kind, route.Algorithm)
		observeRuntimeCompat(CompatibilityHit{Used: true, Source: CompatibilitySourceDraftFormat}, "payload_format")
	}
	return route
}

// ResolveDescriptorKey derives a DescriptorKey for non-frozen routes via registered compatibility sources.
func (r CompatibilityResolver) ResolveDescriptorKey(route ModelRoute) (DescriptorKey, error) {
	if route.HasFrozenRuntime() {
		return DescriptorKey{}, fmt.Errorf("compatibility resolver must not handle frozen runtime identity")
	}
	modelcatalog.ObserveWritePolicy(route.Kind, route.Algorithm)

	family, hit, ok := ExecutionFamilyFromRouteWithCompat(route)
	if !ok {
		return DescriptorKey{}, fmt.Errorf("unsupported model route for runtime descriptor: %s/%s", route.Kind, route.Algorithm)
	}
	observeRuntimeCompat(hit, "family")

	decision, decisionHit := ExecutionDecisionFromRouteWithCompat(route, family)
	if decision == "" {
		return DescriptorKey{}, fmt.Errorf("unable to resolve decision kind for route %s/%s", route.Kind, route.Algorithm)
	}
	observeRuntimeCompat(decisionHit, "decision")

	format := route.PayloadFormat
	if format == "" {
		format = modelcatalog.DraftPayloadFormatForModel(route.Kind, route.Algorithm)
		observeRuntimeCompat(CompatibilityHit{Used: true, Source: CompatibilitySourceDraftFormat}, "payload_format")
	}
	return DescriptorKey{
		AlgorithmFamily: family,
		DecisionKind:    decision,
		PayloadFormat:   format,
	}, nil
}
