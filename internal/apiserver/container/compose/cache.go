package compose

import sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"

func ResolveCacheCapability(provider sharedcache.PolicyProvider, capability sharedcache.Capability) sharedcache.EffectiveCapability {
	if provider == nil {
		return sharedcache.EffectiveCapability{Capability: capability}
	}
	effective, _ := provider.Resolve(capability)
	return effective
}
