package cachepolicy

import sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"

func NewEffectiveRegistry(catalog *PolicyCatalog) *sharedcache.Registry {
	entries := make([]sharedcache.EffectiveCapability, 0, len(specs))
	for _, spec := range specs {
		binding, _ := catalog.Resolve(spec.ID)
		entries = append(entries, sharedcache.EffectiveCapability{
			Capability: spec.ID, Owner: spec.Owner, Kind: spec.Kind, Layer: spec.Layer,
			Family: string(spec.Family), Enabled: binding.Enabled, Policy: binding.Policy,
			Source: spec.ConfigPath, Version: "v2", MetricLabel: spec.MetricLabel,
		})
	}
	return sharedcache.NewRegistry(entries...)
}
