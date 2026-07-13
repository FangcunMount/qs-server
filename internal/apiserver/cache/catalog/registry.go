package cachepolicy

import sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"

func NewEffectiveRegistry(catalog *PolicyCatalog) *sharedcache.Registry {
	entries := make([]sharedcache.EffectiveCapability, 0, len(specs))
	for _, spec := range specs {
		entry, _ := catalog.Effective(spec.ID)
		entries = append(entries, entry)
	}
	return sharedcache.NewRegistry(entries...)
}
